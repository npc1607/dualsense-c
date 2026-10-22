package radio

import (
	"context"
	"time"
)

// RuleClassifier implements deterministic sub-microsecond physical telemetry analysis
// with strict priority hierarchy resolution.
type RuleClassifier struct{}

// NewRuleClassifier creates a new RuleClassifier instance.
func NewRuleClassifier() *RuleClassifier {
	return &RuleClassifier{}
}

// Close closes any resources used by the classifier (no-op for RuleClassifier).
func (c *RuleClassifier) Close() error {
	return nil
}

// Classify evaluates the telemetry window against deterministic driving rules and
// returns the highest priority matching RadioIntent, or nil if no rule triggered.
func (c *RuleClassifier) Classify(ctx context.Context, w *TelemetryWindow) (*RadioIntent, error) {
	if w == nil || w.Count() == 0 {
		return nil, nil
	}

	latest, ok := w.Latest()
	if !ok {
		return nil, nil
	}

	vals := latest.Values

	// 1. Severe Crash / Extreme Hazard (Priority 100)
	accelG := absf(vals["accel_g"])
	speed := vals["speed"]
	if speed > 15 && accelG > 0.45 && accelG < 30 {
		return &RadioIntent{
			ID:       "heavy_crash",
			Category: CategoryWarning,
			Priority: 100,
			ClipName: "heavy_crash",
			Text:     "收到撞击，检查车身受损！",
			Cooldown: 15 * time.Second,
		}, nil
	}

	// 2. Engine Redline / Over-rev (Priority 70)
	if w.DurationAbove("rpm", 7200) >= 1200*time.Millisecond ||
		w.DurationAbove("custom0", 7200) >= 1200*time.Millisecond {
		return &RadioIntent{
			ID:       "engine_redline",
			Category: CategoryMechanical,
			Priority: 70,
			ClipName: "engine_redline",
			Text:     "转速逼近红线，立即升挡！",
			Cooldown: 14 * time.Second,
		}, nil
	}

	// 3. Tire Slip / Loss of Grip (Priority 50)
	slip := vals["slip"]
	slipFL := vals["slip_fl"]
	slipFR := vals["slip_fr"]
	if speed > 20 && (slip > 1.2 || slipFL > 1.5 || slipFR > 1.5) {
		return &RadioIntent{
			ID:       "tire_slip",
			Category: CategoryWarning,
			Priority: 50,
			ClipName: "tire_slip",
			Text:     "注意，后轮严重打滑，稳住方向！",
			Cooldown: 10 * time.Second,
		}, nil
	}

	// 4. Pit / Strategy Call (Priority 40)
	if vals["box"] > 0 || vals["pit"] > 0 {
		return &RadioIntent{
			ID:       "f1_box",
			Category: CategoryStrategy,
			Priority: 40,
			ClipName: "f1_box",
			Text:     "Box this lap, box box.",
			Cooldown: 60 * time.Second,
		}, nil
	}

	// 5. Pace / Clean Flying Lap (Priority 30)
	durPace := w.DurationSustained(func(v map[string]float32) bool {
		return v["speed"] > 115 && v["slip"] < 0.25
	})
	if durPace >= 3500*time.Millisecond {
		return &RadioIntent{
			ID:       "fastest_pace",
			Category: CategoryPace,
			Priority: 30,
			ClipName: "fastest_pace",
			Text:     "漂亮！节奏非常好，继续保持！",
			Cooldown: 45 * time.Second,
		}, nil
	}

	return nil, nil
}
