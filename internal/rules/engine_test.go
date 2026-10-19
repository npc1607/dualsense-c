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
