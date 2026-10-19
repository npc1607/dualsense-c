// Package udp is the telemetry source for games that already broadcast their
// state over UDP: Forza Horizon / Motorsport ("Data Out"), BeamNG.drive and
// Live For Speed (OutGauge), DiRT Rally / EA WRC / F1 (Codemasters style), plus
// a fully declarative field-table decoder so a new game needs no Go code.
//
// No hooking, no memory reading: if the game can send telemetry, this adapter
// can consume it.
package udp

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("udp", New) }

// decoder turns one datagram into signals.
type decoder interface {
	Name() string
	Decode(pkt []byte) (map[string]float32, bool)
}

// Source listens on a UDP port and forwards decoded frames.
type Source struct {
	conn     *net.UDPConn
	dec      decoder
	rate     int
	log      *log.Logger
	dumpOnce map[int]bool
	dump     bool
}

// New builds the source from a profile entry:
//
//	source:
//	  type: udp
//	  listen: "0.0.0.0:5300"
//	  decoder: forza | outgauge | dirt | fields | auto
//	  rate_hz: 60
//	  dump: true            # hexdump each new packet length once
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	if deps.Logger == nil {
		deps.Logger = log.Default()
	}
	listen := "0.0.0.0:5300"
	if v, ok := cfg.Params["listen"].(string); ok && v != "" {
		listen = v
	}
	rate := 60
	if v, ok := cfg.Params["rate_hz"].(float64); ok && v > 0 {
		rate = int(v)
	}
	dec, err := newDecoder(asString(cfg.Params["decoder"]), cfg.Params, deps.Logger)
	if err != nil {
		return nil, err
	}
	addr, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil {
		return nil, fmt.Errorf("udp source: bad listen %q: %w", listen, err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("udp source: listen %q: %w", listen, err)
	}
	dump := false
	if v, ok := cfg.Params["dump"].(bool); ok {
		dump = v
	}
	return &Source{
		conn:     conn,
		dec:      dec,
		rate:     rate,
		log:      deps.Logger,
		dump:     dump,
		dumpOnce: map[int]bool{},
	}, nil
}

func (s *Source) Name() string { return "udp" }

// Run receives datagrams until ctx is cancelled.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	defer s.conn.Close()
	s.log.Printf("udp: listening on %s (decoder %s at %d Hz)", s.conn.LocalAddr(), s.dec.Name(), s.rate)

	// Unblock ReadFrom on cancellation.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.conn.Close()
		case <-done:
		}
	}()

	minGap := time.Second / time.Duration(s.rate)
	buf := make([]byte, 4096)
	var last time.Time
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("udp source: read: %w", err)
		}
		if s.dump && !s.dumpOnce[n] {
			s.dumpOnce[n] = true
			s.log.Printf("udp: packet len=%d\n%s", n, hexDump(buf[:n], 32))
		}
		vals, ok := s.dec.Decode(buf[:n])
		if !ok || len(vals) == 0 {
			continue
		}
		now := time.Now()
		if now.Sub(last) < minGap {
			continue
		}
		last = now
		select {
		case out <- signal.Frame{Values: vals, At: now}:
		case <-ctx.Done():
			return nil
		}
	}
}

func newDecoder(name string, params map[string]any, logger *log.Logger) (decoder, error) {
	switch name {
	case "", "auto":
		var ds []decoder
		for _, n := range []string{"forza", "outgauge"} {
			d, err := newDecoder(n, params, logger)
			if err != nil {
				return nil, err
			}
			ds = append(ds, d)
		}
		if len(asMap(params["fields"])) > 0 {
			d, err := newFieldsDecoder(params, logger)
			if err != nil {
				return nil, err
			}
			ds = append(ds, d)
		}
		return &autoDecoder{decs: ds, logger: logger}, nil
	case "forza":
		return &forzaDecoder{baseDecoder: baseDecoder{minLen: 311}}, nil
	case "outgauge":
		maxRPM := 8000.0
		if v, ok := params["max_rpm"].(float64); ok && v > 0 {
			maxRPM = v
		}
		return &outgaugeDecoder{baseDecoder: baseDecoder{minLen: 92}, maxRPM: maxRPM}, nil
	case "dirt":
		return newDirtDecoder(params, logger)
	case "fields":
		return newFieldsDecoder(params, logger)
	case "acshm", "bridge", "ac", "acc", "acr":
		// Assetto Corsa / ACC / AC Rally via cmd/ds5bridge (shared memory).
		return &bridgeDecoder{}, nil
	}
	return nil, fmt.Errorf("udp source: unknown decoder %q (use forza, outgauge, dirt, acshm, fields or auto)", name)
}

// baseDecoder carries the shared length guard.
type baseDecoder struct{ minLen int }

func (b baseDecoder) accepts(pkt []byte) bool { return len(pkt) >= b.minLen }

// autoDecoder tries each decoder until one claims the packet, then sticks with
// it so a single bad datagram cannot make the signals flap between formats.
type autoDecoder struct {
	decs   []decoder
	sticky decoder
	logger *log.Logger
}

func (a *autoDecoder) Name() string { return "auto" }

func (a *autoDecoder) Decode(pkt []byte) (map[string]float32, bool) {
	if a.sticky != nil {
		if v, ok := a.sticky.Decode(pkt); ok {
			return v, true
		}
	}
	for _, d := range a.decs {
		v, ok := d.Decode(pkt)
		if !ok {
			continue
		}
		if a.sticky == nil {
			a.sticky = d
			a.logger.Printf("udp: locked onto %s decoder (packet len %d)", d.Name(), len(pkt))
		}
		return v, true
	}
	return nil, false
}

func hexDump(b []byte, limit int) string {
	if len(b) > limit {
		b = b[:limit]
	}
	out := make([]byte, 0, len(b)*3)
	for i, c := range b {
		if i > 0 && i%16 == 0 {
			out = append(out, '\n')
		}
		out = append(out, "0123456789abcdef"[c>>4], "0123456789abcdef"[c&0xf], ' ')
	}
	return string(out)
}
