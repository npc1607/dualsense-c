// Package sink owns the output tick. It writes frames at a fixed rate so
// trigger behaviour does not inherit the (variable) telemetry rate, and it
// guarantees the controller is released on shutdown.
package sink

import (
	"context"
	"log"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/report"
)

// Producer computes the frame for a tick.
type Producer func(now time.Time) hid.Frame

// Sink drives one controller.
type Sink struct {
	dev    device.Device
	rate   int
	log    *log.Logger
	reopen func(device.Device) (device.Device, error)
}

// New returns a sink writing at rateHz.
func New(dev device.Device, rateHz int, logger *log.Logger) *Sink {
	if rateHz <= 0 {
		rateHz = 125
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Sink{dev: dev, rate: rateHz, log: logger}
}

// SetReopen installs a recovery hook. It is called when a write fails, which
// on Linux means the controller was unplugged or re-enumerated (the hidraw node
// vanishes and comes back under a new number). Without it the sink would keep
// writing to a dead file descriptor forever.
func (s *Sink) SetReopen(fn func(device.Device) (device.Device, error)) { s.reopen = fn }

// Write sends a single frame.
func (s *Sink) Write(f hid.Frame) error {
	return s.dev.Write(report.Build(s.dev.Info(), f))
}

// Run ticks until ctx is cancelled, then releases the controller.
func (s *Sink) Run(ctx context.Context, produce Producer) error {
	interval := time.Second / time.Duration(s.rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		errCount   int
		lastErrAt  time.Time
		lastOpenAt time.Time
	)
	for {
		select {
		case <-ctx.Done():
			if err := s.Write(hid.Idle()); err != nil {
				s.log.Printf("sink: release failed: %v", err)
			}
			return nil
		case now := <-ticker.C:
			if err := s.Write(produce(now)); err != nil {
				errCount++
				if time.Since(lastErrAt) > 2*time.Second {
					s.log.Printf("sink: write failed (%d so far): %v", errCount, err)
					lastErrAt = time.Now()
				}
				// A re-enumerated controller needs a new fd. Retry at most once a
				// second so a long unplug does not spin.
				if s.reopen != nil && time.Since(lastOpenAt) > time.Second {
					lastOpenAt = time.Now()
					if dev, err := s.reopen(s.dev); err == nil && dev != nil {
						s.dev = dev
						errCount = 0
						s.log.Printf("sink: reattached to %s", dev.Info())
					}
				}
			}
		}
	}
}
