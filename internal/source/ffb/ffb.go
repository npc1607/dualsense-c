// Package ffb consumes force-feedback state captured by a virtual gamepad and
// exposes it as signals.
//
// A game only sends rumble to the pad it is talking to, so the capture has to
// happen in the virtual-pad backend (see internal/vpad and cmd/ds5vpad). This
// package is deliberately just a wire protocol: the backend publishes two bytes
// per update, and any future platform backend can feed the same port.
//
// Packet layout (little endian):
//
//	0  magic  "FFB1" (uint32 0x31424646)
//	4  left   0..255
//	5  right  0..255
//	6  _      reserved
//	7  _      reserved
//	8  seq    uint32, optional monotonic counter
package ffb

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("ffb", New) }

// Magic identifies a force-feedback datagram.
const Magic uint32 = 0x31424646 // "FFB1"

// DefaultListen is where ds5vpad publishes by default.
const DefaultListen = "127.0.0.1:9998"

// PacketBytes is the fixed datagram size.
const PacketBytes = 12

// Source listens for capture packets and republishes them as signals.
type Source struct {
	addr string
	log  *log.Logger
}

// New builds the source. Profile parameters:
//
//	listen  UDP address to bind (default 127.0.0.1:9998)
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	addr := DefaultListen
	if v, ok := cfg.Params["listen"]; ok {
		if s, ok := v.(string); ok && s != "" {
			addr = s
		}
	}
	logger := deps.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Source{addr: addr, log: logger}, nil
}

func (s *Source) Name() string { return "ffb" }

// Run listens until ctx ends and forwards every packet as one frame.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", s.addr)
	if err != nil {
		return fmt.Errorf("ffb: %w", err)
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("ffb: listen %s: %w", s.addr, err)
	}
	defer conn.Close()
	s.log.Printf("ffb: listening on %s for virtual-pad rumble", s.addr)

	go func() {
		<-ctx.Done()
		_ = conn.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, 64)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return nil
			}
			return fmt.Errorf("ffb: read: %w", err)
		}
		left, right, ok := decode(buf[:n])
		if !ok {
			continue
		}
		select {
		case out <- signal.Frame{
			At: time.Now(),
			Values: map[string]float32{
				"ffb_left":  left,
				"ffb_right": right,
			},
		}:
		case <-ctx.Done():
			return nil
		}
	}
}

func decode(b []byte) (left, right float32, ok bool) {
	if len(b) < 6 || binary.LittleEndian.Uint32(b) != Magic {
		return 0, 0, false
	}
	return float32(b[4]) / 255, float32(b[5]) / 255, true
}

// Encode builds a capture packet; used by virtual-pad backends and tests.
func Encode(left, right uint8, seq uint32) []byte {
	b := make([]byte, PacketBytes)
	binary.LittleEndian.PutUint32(b[0:], Magic)
	b[4] = left
	b[5] = right
	binary.LittleEndian.PutUint32(b[8:], seq)
	return b
}
