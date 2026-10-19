// Package audioai implements a source adapter that ingests sound event and
// surface classifications from an external lightweight AI detector
// (e.g. YAMNet ONNX running via local loopback capture).
package audioai

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() {
	source.Register("audio_ai", New)
}

const (
	defaultListenAddr = "127.0.0.1:9998"
	binaryMagic       = 0x41355344 // "DS5A" in little-endian (DualSense Audio AI)
)

// Source listens on UDP for audio event detections and publishes them as signals.
type Source struct {
	listenAddr string
	log        *log.Logger
}

// New creates an Audio AI source adapter.
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	addr := defaultListenAddr
	if v, ok := cfg.Params["listen"].(string); ok && v != "" {
		addr = v
	}
	logger := deps.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Source{
		listenAddr: addr,
		log:        logger,
	}, nil
}

func (s *Source) Name() string {
	return "audio_ai"
}

// Run listens for incoming detection packets and emits signal frames.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	addr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("audio_ai: resolve %s: %w", s.listenAddr, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("audio_ai: listen %s: %w", s.listenAddr, err)
	}
	defer conn.Close()

	// Ensure socket is closed when context is cancelled so ReadFromUDP unblocks.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	s.log.Printf("audio_ai: listening for detector packets on %s", s.listenAddr)

	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("audio_ai: read: %w", err)
		}
		if n == 0 {
			continue
		}

		vals, ok := decodePacket(buf[:n])
		if !ok || len(vals) == 0 {
			continue
		}

		frame := signal.Frame{
			At:     time.Now(),
			Values: vals,
		}

		select {
		case out <- frame:
		case <-ctx.Done():
			return nil
		}
	}
}

// decodePacket parses either a JSON dictionary or a compact binary frame.
func decodePacket(b []byte) (map[string]float32, bool) {
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]float32
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, false
		}
		// Validate and filter against standard signals
		out := make(map[string]float32, len(raw))
		for k, v := range raw {
			if signal.IsStandard(k) {
				out[k] = v
			}
		}
		return out, true
	}

	// Binary frame protocol:
	// 0..3: magic "DS5A" (0x41355344)
	// 4..5: version (uint16 = 1)
	// 6:    count (uint8)
	// 7:    reserved
	// 8..:  count * { id uint8, value float32 }
	if len(b) >= 8 && binary.LittleEndian.Uint32(b[:4]) == binaryMagic {
		count := int(b[6])
		if len(b) < 8+count*5 {
			return nil, false
		}
		out := make(map[string]float32, count)
		offset := 8
		for i := 0; i < count; i++ {
			id := b[offset]
			valBits := binary.LittleEndian.Uint32(b[offset+1 : offset+5])
			offset += 5

			name := signal.Name(id)
			if name != "" {
				out[name] = math.Float32frombits(valBits)
			}
		}
		return out, true
	}

	return nil, false
}
