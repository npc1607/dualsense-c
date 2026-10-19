// Package sim is a device-free source that replays a scripted trigger pattern.
// It exists so profiles can be developed and regression-tested without a
// controller or a game (`ds5d -dry-run -source sim`).
package sim

import (
	"context"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("sim", New) }

// Source emits a repeating 8 second script.
type Source struct {
	rate int
}

// New builds the simulator.
func New(cfg profile.Source, _ source.Deps) (source.Source, error) {
	rate := 125
	if v, ok := cfg.Params["rate_hz"].(float64); ok && v > 0 {
		rate = int(v)
	}
	return &Source{rate: rate}, nil
}

func (s *Source) Name() string { return "sim" }

// Script (seconds into an 8s cycle):
//
//	0.0-0.5  rest
//	0.5-2.0  L2 ramps up to 0.5 (aim wall territory)
//	2.0-3.0  L2 held at 0.5
//	3.0-3.6  L2 pushes through to 1.0
//	3.6-4.0  L2 released
//	4.0-5.6  R2 ramps 0 -> 1 (crosses the fire point at 0.85)
//	5.6-6.2  R2 held
//	6.2-6.6  R2 released
//	6.6-8.0  rest
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	ticker := time.NewTicker(time.Second / time.Duration(s.rate))
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			t := now.Sub(start).Seconds()
			for t >= 8 {
				t -= 8
			}
			l2, r2 := script(t)
			frame := signal.Frame{At: now, Values: map[string]float32{
				"l2":      float32(l2),
				"r2":      float32(r2),
				"l2_full": bool01(l2 >= 0.85),
				"r2_full": bool01(r2 >= 0.85),
			}}
			select {
			case out <- frame:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func script(t float64) (l2, r2 float64) {
	switch {
	case t < 0.5:
	case t < 2.0:
		l2 = 0.5 * (t - 0.5) / 1.5
	case t < 3.0:
		l2 = 0.5
	case t < 3.6:
		l2 = 0.5 + 0.5*(t-3.0)/0.6
	case t < 4.0:
		l2 = 1 - (t-3.6)/0.4
	case t < 5.6:
		r2 = (t - 4.0) / 1.6
	case t < 6.2:
		r2 = 1
	case t < 6.6:
		r2 = 1 - (t-6.2)/0.4
	}
	if l2 < 0 {
		l2 = 0
	}
	if r2 < 0 {
		r2 = 0
	}
	return l2, r2
}

func bool01(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
