package radio

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
	_ "github.com/npc1607/dualsense-c/internal/source/all"
	"github.com/npc1607/dualsense-c/internal/source/udp"
)

// Trigger defines a condition under which a radio clip is broadcast.
type Trigger struct {
	Name     string
	Clip     string
	Cooldown time.Duration
	Evaluate func(vals map[string]float32, dt time.Duration, state *TriggerState) bool
}

// TriggerState tracks persistence and timing for evaluation.
type TriggerState struct {
	HoldDuration time.Duration
	LastFired    time.Time
}

// Monitor watches live telemetry and broadcasts engineer radio lines.
type Monitor struct {
	player   *Player
	triggers []Trigger
	states   map[string]*TriggerState
	logger   *log.Logger
	mu       sync.Mutex
}

// NewMonitor initializes a telemetry radio monitor.
func NewMonitor(player *Player, logger *log.Logger) *Monitor {
	if logger == nil {
		logger = log.Default()
	}
	m := &Monitor{
		player: player,
		logger: logger,
		states: make(map[string]*TriggerState),
	}
	m.triggers = DefaultTriggers()
	return m
}

// DefaultTriggers returns the standard motorsport race engineer rules.
func DefaultTriggers() []Trigger {
	return []Trigger{
		{
			Name:     "tire_slip",
			Clip:     "tire_slip",
			Cooldown: 10 * time.Second,
			Evaluate: func(vals map[string]float32, _ time.Duration, _ *TriggerState) bool {
				speed := vals["speed"]
				slip := vals["slip"]
				slipFront := maxf(vals["slip_fl"], vals["slip_fr"])
				return speed > 20 && (slip > 1.2 || slipFront > 1.5)
			},
		},
		{
			Name:     "engine_redline",
			Clip:     "engine_redline",
			Cooldown: 14 * time.Second,
			Evaluate: func(vals map[string]float32, dt time.Duration, s *TriggerState) bool {
				rpm := vals["custom0"]
				if rpm > 7200 {
					s.HoldDuration += dt
				} else {
					s.HoldDuration = 0
				}
				// Must hold above 7200 RPM for 1.2 seconds to confirm overdue shift
				return s.HoldDuration >= 1200*time.Millisecond
			},
		},
		{
			Name:     "heavy_crash",
			Clip:     "heavy_crash",
			Cooldown: 15 * time.Second,
			Evaluate: func(vals map[string]float32, _ time.Duration, _ *TriggerState) bool {
				accelG := absf(vals["accel_g"])
				speed := vals["speed"]
				return speed > 15 && accelG > 0.45 && accelG < 30
			},
		},
		{
			Name:     "fastest_pace",
			Clip:     "fastest_pace",
			Cooldown: 45 * time.Second,
			Evaluate: func(vals map[string]float32, dt time.Duration, s *TriggerState) bool {
				speed := vals["speed"]
				slip := vals["slip"]
				if speed > 115 && slip < 0.25 {
					s.HoldDuration += dt
				} else {
					s.HoldDuration = 0
				}
				// 3.5 seconds of high-speed clean run
				return s.HoldDuration >= 3500*time.Millisecond
			},
		},
	}
}

// Run listens on a UDP telemetry address and evaluates triggers until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context, listenAddr string, decoderName string) error {
	m.logger.Printf("radio/monitor: listening on %s (decoder: %s)", listenAddr, decoderName)

	src, err := udp.New(profile.Source{
		Type: "udp",
		Params: map[string]any{
			"listen":  listenAddr,
			"decoder": decoderName,
			"rate_hz": 60.0,
		},
	}, source.Deps{Logger: m.logger})
	if err != nil {
		return fmt.Errorf("radio/monitor: create udp source: %w", err)
	}

	frames := make(chan signal.Frame, 64)
	go func() {
		if err := src.Run(ctx, frames); err != nil && ctx.Err() == nil {
			m.logger.Printf("radio/monitor: source stopped: %v", err)
		}
	}()

	var lastTime time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case f, ok := <-frames:
			if !ok {
				return nil
			}
			now := f.At
			dt := 16 * time.Millisecond
			if !lastTime.IsZero() {
				dt = now.Sub(lastTime)
			}
			lastTime = now

			m.processFrame(ctx, f.Values, dt, now)
		}
	}
}

func (m *Monitor) processFrame(ctx context.Context, vals map[string]float32, dt time.Duration, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, trig := range m.triggers {
		st, ok := m.states[trig.Name]
		if !ok {
			st = &TriggerState{}
			m.states[trig.Name] = st
		}

		if now.Sub(st.LastFired) < trig.Cooldown {
			continue
		}

		if trig.Evaluate(vals, dt, st) {
			st.LastFired = now
			st.HoldDuration = 0
			m.logger.Printf("radio/monitor: EVENT [%s] -> broadcasting clip %q", trig.Name, trig.Clip)
			go func(clip string) {
				_ = m.player.Play(context.Background(), clip)
			}(trig.Clip)
			break // Only fire one radio message at a time
		}
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
