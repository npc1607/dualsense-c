// Package trigger is the input-driven source: it reads the DualSense's own
// input reports and exposes trigger travel, buttons and sticks as signals.
// It needs no game integration at all, which is why it works in every game.
package trigger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/input"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("trigger", New) }

// Source streams controller input as signals.
type Source struct {
	dev  device.Device
	rate int
	log  *log.Logger
}

// New builds the source. It requires an open device: there is nothing to read
// in -dry-run mode.
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	if deps.Device == nil {
		return nil, errors.New("trigger source: no device (drop -dry-run, or use a source that does not need one)")
	}
	if !deps.Device.Readable() {
		return nil, fmt.Errorf("trigger source: %s does not support input reports on this platform", deps.Device.Info().Path)
	}
	rate := 125
	if v, ok := cfg.Params["rate_hz"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			rate = int(f)
		}
	}
	if deps.Logger == nil {
		deps.Logger = log.Default()
	}
	return &Source{dev: deps.Device, rate: rate, log: deps.Logger}, nil
}

func (s *Source) Name() string { return "trigger" }

// Run reads input reports and emits frames at no more than rate Hz.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	minGap := time.Second / time.Duration(s.rate)
	buf := make([]byte, 128)
	var last time.Time
	read := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		n, err := s.dev.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("trigger source: read: %w", err)
		}
		st, ok := input.Parse(buf[:n])
		if !ok {
			continue
		}
		read++
		now := time.Now()
		if now.Sub(last) < minGap {
			continue
		}
		last = now
		select {
		case out <- signal.Frame{Values: st.Signals(), At: now}:
		case <-ctx.Done():
			return nil
		}
	}
}
