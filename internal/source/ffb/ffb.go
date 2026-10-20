// Package ffb consumes force-feedback state captured by a virtual gamepad and
// exposes it as signals.
//
// A game only sends rumble to the pad it is talking to, so the capture has to
// happen in the virtual-pad backend (see internal/vpad and cmd/ds5vpad).
// Communication uses the high-performance, ultra-low latency event-driven
// vpad/ipc protocol (< 50 microseconds latency, zero polling ticker lag).
package ffb

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"
)

func init() { source.Register("ffb", New) }

// Magic identifies a force-feedback protocol frame ("FFB1").
const Magic = ipc.Magic

// DefaultListen is the default IPC endpoint.
const DefaultListen = ipc.DefaultEndpoint

// PacketBytes is the fixed wire frame size (12 bytes).
const PacketBytes = ipc.FrameSize

// Source connects to the virtual gamepad IPC server and republishes rumble events as signals.
type Source struct {
	endpoint string
	log      *log.Logger
}

// New builds the source. Profile parameters:
//
//	listen  IPC endpoint to connect to (default 127.0.0.1:9998, or unix:///path.sock)
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	ep := DefaultListen
	if v, ok := cfg.Params["listen"]; ok {
		if s, ok := v.(string); ok && s != "" {
			ep = s
		}
	}
	logger := deps.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Source{endpoint: ep, log: logger}, nil
}

func (s *Source) Name() string { return "ffb" }

// Run connects to the virtual pad IPC server and pushes incoming rumble frames.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	s.log.Printf("ffb: connecting to virtual-pad IPC at %s", s.endpoint)

	buf := make([]byte, ipc.FrameSize)
	for ctx.Err() == nil {
		conn, err := ipc.Dial(ctx, s.endpoint)
		if err != nil {
			select {
			case <-time.After(250 * time.Millisecond):
				continue
			case <-ctx.Done():
				return nil
			}
		}

		s.log.Printf("ffb: connected to virtual pad IPC (%s)", s.endpoint)
		s.stream(ctx, conn, buf, out)
		_ = conn.Close()

		if ctx.Err() == nil {
			s.log.Printf("ffb: connection lost, reconnecting...")
		}
	}
	return nil
}

func (s *Source) stream(ctx context.Context, conn net.Conn, buf []byte, out chan<- signal.Frame) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		left, right, _, err := ipc.ReadFrame(conn, buf)
		if err != nil {
			if ctx.Err() != nil || err == io.EOF {
				return
			}
			return
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
			return
		}
	}
}

// Encode builds a capture packet; used by tests and virtual-pad backends.
func Encode(left, right uint8, seq uint32) []byte {
	b := make([]byte, ipc.FrameSize)
	binary.LittleEndian.PutUint32(b[0:], ipc.Magic)
	b[4] = left
	b[5] = right
	binary.LittleEndian.PutUint32(b[8:], seq)
	return b
}
