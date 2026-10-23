// Package sink owns the output tick. It writes frames at a fixed rate so
// trigger behaviour does not inherit the (variable) telemetry rate, and it
// guarantees the controller is released on shutdown.
package sink

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/report"
)

// Producer computes the frame for a tick.
type Producer func(now time.Time) hid.Frame

// Stats is a point-in-time snapshot of the sink counters.
type Stats struct {
	WriteErrors uint64 `json:"write_errors"`
	Reconnects  uint64 `json:"reconnects"`
}

// Counters accumulates cumulative output statistics. The zero value is ready
// to use. The sink goroutine increments them while CMD1 handlers read them via
// Snapshot, so every field must stay atomic.
type Counters struct {
	writeErrors atomic.Uint64
	reconnects  atomic.Uint64
}

// NewCounters returns an empty counter set.
func NewCounters() *Counters { return &Counters{} }

// Snapshot returns the current values. A nil *Counters is treated as zero so
// callers can share an optional pointer without nil checks.
func (c *Counters) Snapshot() Stats {
	if c == nil {
		return Stats{}
	}
	return Stats{
		WriteErrors: c.writeErrors.Load(),
		Reconnects:  c.reconnects.Load(),
	}
}

// Sink drives one controller.
type Sink struct {
	dev      device.Device
	rate     int
	log      *log.Logger
	reopen   func(device.Device) (device.Device, error)
	counters *Counters
}

// New returns a sink writing at rateHz.
func New(dev device.Device, rateHz int, logger *log.Logger) *Sink {
	if rateHz <= 0 {
		rateHz = 125
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Sink{dev: dev, rate: rateHz, log: logger, counters: NewCounters()}
}

// SetCounters shares a counter set with the daemon so its health endpoint can
// expose cumulative write errors and reconnects. Call it before Run; passing
// nil keeps the sink's private counters.
func (s *Sink) SetCounters(c *Counters) {
	if c != nil {
		s.counters = c
	}
}

// Stats returns a race-free snapshot of the sink counters.
func (s *Sink) Stats() Stats { return s.counters.Snapshot() }

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
		errCount   int // consecutive errors since the last reattach, for log noise only
		lastErrAt  time.Time
		lastOpenAt time.Time
	)
	for {
		select {
		case <-ctx.Done():
			if err := s.Write(hid.Idle()); err != nil {
				s.counters.writeErrors.Add(1)
				s.log.Printf("sink: release failed: %v", err)
			}
			return nil
		case now := <-ticker.C:
			if err := s.Write(produce(now)); err != nil {
				errCount++
				s.counters.writeErrors.Add(1)
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
						s.counters.reconnects.Add(1)
						s.log.Printf("sink: reattached to %s", dev.Info())
					}
				}
			}
		}
	}
}
