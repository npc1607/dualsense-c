package rules

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
)

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func mustEngine(t *testing.T, p *profile.Profile, set *signal.Set) *Engine {
	t.Helper()
	e, err := New(p, set, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func apply(set *signal.Set, at time.Time, kv map[string]float32) {
	set.Apply(signal.Frame{At: at, Values: kv})
}

func TestRigidForceFollowsSignal(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			Name:   "l2",
			Target: "l2",
			Effect: "rigid",
			Params: map[string]string{"start": "0", "force": "clamp(20 + 235*brake, 0, 255)"},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"brake": 1})
	f := e.Step(base.Add(time.Millisecond))
	if f.L2.Mode != hid.ModeRigid {
		t.Fatalf("mode = %v", f.L2.Mode)
	}
	if f.L2.P[1] != 255 {
		t.Errorf("force = %d, want 255", f.L2.P[1])
	}
}

func TestRuleIsGatedByWhen(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{
			{Name: "wall", When: "l2 > 0.1 && l2 < 0.5", Target: "l2", Effect: "feedback",
				Params:   map[string]string{"start": "0", "strength": "100", "force": "120"},
				Priority: 10},
			{Name: "off", When: "l2 >= 0.5", Target: "l2", Effect: "off", Priority: 20},
		},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"l2": 0.3})
	if f := e.Step(base.Add(time.Millisecond)); f.L2.Mode != hid.ModeFeedback {
		t.Errorf("inside window: mode = %v, want feedback", f.L2.Mode)
	}
	apply(set, base.Add(10*time.Millisecond), map[string]float32{"l2": 0.7})
	if f := e.Step(base.Add(11 * time.Millisecond)); !f.L2.IsOff() {
		t.Errorf("past window: mode = %v, want off (higher priority wins)", f.L2.Mode)
	}
}

func TestHoldLatchesPulse(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			Name:   "recoil",
			When:   `pressed("r2")`,
			Target: "rumble_right",
			Value:  "200",
			Hold:   profile.Duration(100 * time.Millisecond),
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"r2": 0})
	if f := e.Step(base); f.RumbleR != 0 {
		t.Fatalf("idle rumble = %d", f.RumbleR)
	}
	apply(set, base.Add(10*time.Millisecond), map[string]float32{"r2": 0.6})
	if f := e.Step(base.Add(10 * time.Millisecond)); f.RumbleR != 200 {
		t.Fatalf("on press: rumble = %d, want 200", f.RumbleR)
	}
	// Signal stays high: predicate is no longer true, but the latch holds.
	apply(set, base.Add(20*time.Millisecond), map[string]float32{"r2": 0.6})
	if f := e.Step(base.Add(50 * time.Millisecond)); f.RumbleR != 200 {
		t.Errorf("during hold: rumble = %d, want 200", f.RumbleR)
	}
	// The first inactive tick starts the fade (still full strength) ...
	e.Step(base.Add(120 * time.Millisecond))
	// ... and the next ones ramp it down instead of snapping to zero.
	if f := e.Step(base.Add(200 * time.Millisecond)); f.RumbleR == 0 || f.RumbleR >= 200 {
		t.Errorf("after hold: rumble = %d, want a value between 1 and 199", f.RumbleR)
	}
}

func TestReleaseDecayThenOff(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Safety: profile.Safety{
			SignalTTL: profile.Duration(10 * time.Millisecond),
			Release:   profile.Duration(100 * time.Millisecond),
		},
		Rules: []profile.Rule{{
			When:   `has("brake")`,
			Target: "l2", Effect: "rigid",
			Params: map[string]string{"force": "200"},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"brake": 0})
	f := e.Step(base)
	if f.L2.P[1] != 200 {
		t.Fatalf("force = %d", f.L2.P[1])
	}

	// Signals expire -> the rule stops -> the trigger fades over Release.
	// First inactive tick starts the fade at full strength.
	e.Step(base.Add(50 * time.Millisecond))
	f = e.Step(base.Add(100 * time.Millisecond))
	if f.L2.P[1] == 0 || f.L2.P[1] >= 200 {
		t.Errorf("mid-release force = %d, want partial", f.L2.P[1])
	}
	f = e.Step(base.Add(200 * time.Millisecond))
	if !f.L2.IsOff() {
		t.Errorf("after release: mode = %v, want off", f.L2.Mode)
	}
}

func TestHasTracksFreshness(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Safety: profile.Safety{
			SignalTTL: profile.Duration(50 * time.Millisecond),
			Release:   profile.Duration(10 * time.Millisecond),
		},
		Rules: []profile.Rule{{
			When: `has("brake")`, Target: "l2", Effect: "rigid",
			Params: map[string]string{"force": "150"},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"brake": 0.5})
	if f := e.Step(base.Add(time.Millisecond)); f.L2.IsOff() {
		t.Error("fresh signal: rule should be active")
	}
	// The first stale tick only starts the fade; the next one is released.
	e.Step(base.Add(200 * time.Millisecond))
	if f := e.Step(base.Add(250 * time.Millisecond)); !f.L2.IsOff() {
		t.Error("stale signal: rule should be inactive")
	}
}

func TestCompileRejectsTypos(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			Target: "l2", Effect: "rigid",
			Params: map[string]string{"force": "clamp(20 + 235*breaks, 0, 255)"},
		}},
	}
	if _, err := New(p, signal.NewSet(nil), testLogger()); err == nil {
		t.Fatal("unknown signal name must fail at compile time")
	}

	p.Rules[0].Params = map[string]string{"bogus": "1"}
	if _, err := New(p, signal.NewSet(nil), testLogger()); err == nil {
		t.Fatal("unknown effect parameter must fail at compile time")
	}
}

func TestLightbarRule(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			Target: "lightbar",
			Params: map[string]string{"r": "255", "g": "0", "b": "128"},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	f := e.Step(time.Now())
	if f.Lightbar != [3]uint8{255, 0, 128} {
		t.Errorf("lightbar = %v", f.Lightbar)
	}
}

func TestMuteLEDTarget(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			When:   `held("mute")`,
			Target: "mute_led",
			Value:  "1",
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"mute": 1})
	if f := e.Step(base.Add(time.Millisecond)); !f.MuteLED {
		t.Error("mute LED should be on while the mute button is held")
	}
	apply(set, base.Add(10*time.Millisecond), map[string]float32{"mute": 0})
	if f := e.Step(base.Add(11 * time.Millisecond)); f.MuteLED {
		t.Error("mute LED should be off after release")
	}
}

func TestSensorSignalsReachRules(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "trigger"},
		Rules: []profile.Rule{{
			When:   "battery > 0 && battery < 0.2",
			Target: "lightbar",
			Params: map[string]string{"r": "255", "g": "0", "b": "0"},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"battery": 0.1})
	if f := e.Step(base.Add(time.Millisecond)); f.Lightbar != [3]uint8{255, 0, 0} {
		t.Errorf("low battery lightbar = %v", f.Lightbar)
	}
	apply(set, base.Add(10*time.Millisecond), map[string]float32{"battery": 0.8})
	if f := e.Step(base.Add(11 * time.Millisecond)); f.Lightbar != [3]uint8{} {
		t.Errorf("battery fine: lightbar = %v, want untouched", f.Lightbar)
	}
}

func TestSinCosAndChargingBreath(t *testing.T) {
	p := &profile.Profile{
		Name:   "charging-test",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			When:   "charging == 1",
			Target: "lightbar",
			Params: map[string]string{
				"r": "0",
				"g": "round(lerp(20, 100, (sin(sec()) + 1) / 2))",
				"b": "round(lerp(50, 200, (cos(sec()) + 1) / 2))",
			},
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	apply(set, base, map[string]float32{"charging": 1})
	f := e.Step(base.Add(time.Millisecond))
	if f.Lightbar[1] == 0 || f.Lightbar[2] == 0 {
		t.Errorf("charging breath lightbar expected non-zero green/blue, got %v", f.Lightbar)
	}
}

// TestAbsfAvoidsAbsSignalCollision guards the naming trap: "abs" is the
// ABS-intervention signal, so absolute value must be reached through absf().
func TestAbsfAvoidsAbsSignalCollision(t *testing.T) {
	p := &profile.Profile{
		Name:   "t",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{{
			Name:   "r",
			When:   "absf(accel_g) > 0.5",
			Target: "rumble_left",
			Value:  "min(absf(accel_g), 255)",
		}},
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()
	apply(set, base, map[string]float32{"accel_g": -1.5})
	if f := e.Step(base.Add(time.Millisecond)); f.RumbleL == 0 {
		t.Errorf("absf(-1.5) should be > 0.5 and drive rumble, got %d", f.RumbleL)
	}
}

// TestBuiltinAssettoCorsaRoadFeel checks that the shipped AC/ACC/ACR profile
// reacts to chassis vertical acceleration: quiet on flat asphalt, buzzing on a
// kerb, and hammering on an impact.
func TestBuiltinAssettoCorsaRoadFeel(t *testing.T) {
	p, err := profile.Resolve("assetto-corsa", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	set := signal.NewSet(nil)
	e := mustEngine(t, p, set)
	base := time.Now()

	// Split of duties: the right (small, high frequency) motor carries the fine
	// texture, the left (large, low frequency) motor stays silent until
	// something actually happens.
	flat := map[string]float32{"speed": 80, "accel_g": 0.01, "throttle": 0.5}
	apply(set, base, flat)
	ff := e.Step(base.Add(time.Millisecond))
	if ff.RumbleR == 0 || ff.RumbleR > 70 {
		t.Errorf("flat asphalt: right rumble = %d, want a low non-zero texture", ff.RumbleR)
	}
	if ff.RumbleL != 0 {
		t.Errorf("flat asphalt: left rumble = %d, want 0 (left only on events)", ff.RumbleL)
	}

	kerb := map[string]float32{"speed": 80, "accel_g": 1.4}
	apply(set, base.Add(20*time.Millisecond), kerb)
	kf := e.Step(base.Add(21 * time.Millisecond))
	if kf.RumbleL == 0 || kf.RumbleR == 0 {
		t.Fatalf("kerb: rumble = %d/%d, want both non-zero", kf.RumbleL, kf.RumbleR)
	}

	impact := map[string]float32{"speed": 80, "accel_g": 4.0}
	apply(set, base.Add(40*time.Millisecond), impact)
	if_ := e.Step(base.Add(41 * time.Millisecond))
	if if_.RumbleL <= kf.RumbleL || if_.RumbleR <= kf.RumbleR {
		t.Errorf("impact should hit harder than kerb: kerb %d/%d -> impact %d/%d",
			kf.RumbleL, kf.RumbleR, if_.RumbleL, if_.RumbleR)
	}

	// Off-track dirt (tyreDirtyLevel) alone must also produce rumble.
	off := map[string]float32{"speed": 40, "surface": 1.0}
	apply(set, base.Add(60*time.Millisecond), off)
	if f := e.Step(base.Add(61 * time.Millisecond)); f.RumbleL == 0 || f.RumbleR == 0 {
		t.Errorf("off-track: rumble = %d/%d, want both non-zero", f.RumbleL, f.RumbleR)
	}
}
