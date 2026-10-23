package main

import (
	"encoding/json"
	"io"
	"log"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/rules"
	sig "github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/sink"
)

func newTestProfile() *profile.Profile {
	return &profile.Profile{
		Name:   "test",
		Source: profile.Source{Type: "sim"},
	}
}

func TestApplyShapePatch_JSONTagAndMerge(t *testing.T) {
	p := newTestProfile()
	set := sig.NewSet(map[string]sig.Shaper{
		"l2": {Deadzone: 0.1},
		"r2": {Gamma: 1.5},
	})

	applied, ignored, reasons, err := applyShapePatch(p.Extra, set,
		json.RawMessage(`{"shape":{"l2":{"slew_up":10},"r2":{"deadzone":0.2}}}`))
	if err != nil {
		t.Fatalf("applyShapePatch failed: %v", err)
	}
	if !reflect.DeepEqual(applied, []string{"l2", "r2"}) {
		t.Fatalf("applied = %v, want [l2 r2]", applied)
	}
	if len(ignored) != 0 || len(reasons) != 0 {
		t.Fatalf("ignored = %v, reasons = %v, want both empty", ignored, reasons)
	}

	l2, _ := set.GetShaper("l2")
	if l2.Deadzone != 0.1 {
		t.Fatalf("l2.Deadzone = %v, want 0.1 preserved (field not in patch)", l2.Deadzone)
	}
	if l2.SlewUp != 10 {
		t.Fatalf("l2.SlewUp = %v, want 10 (snake_case json tag must be honoured)", l2.SlewUp)
	}

	r2, _ := set.GetShaper("r2")
	if r2.Gamma != 1.5 {
		t.Fatalf("r2.Gamma = %v, want 1.5 preserved (field not in patch)", r2.Gamma)
	}
	if r2.Deadzone != 0.2 {
		t.Fatalf("r2.Deadzone = %v, want 0.2", r2.Deadzone)
	}
}

func TestApplyShapePatch_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"nil body", nil, "shape"},
		{"empty body", json.RawMessage{}, "shape"},
		{"invalid json", json.RawMessage(`{"shape":`), "invalid params"},
		{"no shape", json.RawMessage(`{}`), "at least one signal"},
		{"empty shape", json.RawMessage(`{"shape":{}}`), "at least one signal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := sig.NewSet(map[string]sig.Shaper{"l2": {Deadzone: 0.4}})
			applied, ignored, reasons, err := applyShapePatch(newTestProfile().Extra, set, tc.raw)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
			if applied != nil || ignored != nil || reasons != nil {
				t.Fatalf("nothing should be applied on error: applied=%v ignored=%v reasons=%v",
					applied, ignored, reasons)
			}
			if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.4 {
				t.Fatalf("shaper mutated on error: %+v", sh)
			}
		})
	}
}

func TestApplyShapePatch_UnknownAndExtraSignals(t *testing.T) {
	p := newTestProfile()
	set := sig.NewSet(nil)

	applied, ignored, reasons, err := applyShapePatch(p.Extra, set,
		json.RawMessage(`{"shape":{"nope":{"gamma":1}}}`))
	if err != nil {
		t.Fatalf("applyShapePatch failed: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("applied = %v, want empty", applied)
	}
	if !reflect.DeepEqual(ignored, []string{"nope"}) {
		t.Fatalf("ignored = %v, want [nope]", ignored)
	}
	if reasons["nope"] != "unknown signal" {
		t.Fatalf("reasons[nope] = %q, want \"unknown signal\"", reasons["nope"])
	}

	// Signals declared in the profile's `signals:` extension list are accepted.
	p.Extra = []string{"custom_xyz"}
	applied, ignored, _, err = applyShapePatch(p.Extra, set,
		json.RawMessage(`{"shape":{"custom_xyz":{"gamma":1.2}}}`))
	if err != nil {
		t.Fatalf("applyShapePatch failed: %v", err)
	}
	if !reflect.DeepEqual(applied, []string{"custom_xyz"}) {
		t.Fatalf("applied = %v, want [custom_xyz]", applied)
	}
	if len(ignored) != 0 {
		t.Fatalf("ignored = %v, want empty", ignored)
	}
}

func TestApplyShapePatch_OutOfRangeValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"deadzone too big", `{"shape":{"l2":{"deadzone":2}}}`, "deadzone must be between 0 and 1"},
		{"deadzone negative", `{"shape":{"l2":{"deadzone":-0.1}}}`, "deadzone must be between 0 and 1"},
		{"gamma zero", `{"shape":{"l2":{"gamma":0}}}`, "gamma must be > 0"},
		{"gamma negative", `{"shape":{"l2":{"gamma":-1}}}`, "gamma must be > 0"},
		{"slew_up negative", `{"shape":{"l2":{"slew_up":-1}}}`, "slew_up must be >= 0"},
		{"slew_down negative", `{"shape":{"l2":{"slew_down":-1}}}`, "slew_down must be >= 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := sig.NewSet(map[string]sig.Shaper{"l2": {Deadzone: 0.4, Gamma: 1.1}})
			applied, ignored, reasons, err := applyShapePatch(newTestProfile().Extra, set, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("applyShapePatch returned error: %v", err)
			}
			if len(applied) != 0 {
				t.Fatalf("applied = %v, want empty", applied)
			}
			if !reflect.DeepEqual(ignored, []string{"l2"}) {
				t.Fatalf("ignored = %v, want [l2]", ignored)
			}
			if reasons["l2"] != tc.want {
				t.Fatalf("reasons[l2] = %q, want %q", reasons["l2"], tc.want)
			}
			if sh, _ := set.GetShaper("l2"); sh != (sig.Shaper{Deadzone: 0.4, Gamma: 1.1}) {
				t.Fatalf("shaper mutated despite rejection: %+v", sh)
			}
		})
	}
}

func TestApplyShapePatch_MixedValidAndInvalid(t *testing.T) {
	set := sig.NewSet(map[string]sig.Shaper{"r2": {Gamma: 1.5}})

	applied, ignored, reasons, err := applyShapePatch(newTestProfile().Extra, set,
		json.RawMessage(`{"shape":{"l2":{"deadzone":0.2},"r2":{"gamma":0},"nope":{"gamma":1}}}`))
	if err != nil {
		t.Fatalf("applyShapePatch failed: %v", err)
	}
	if !reflect.DeepEqual(applied, []string{"l2"}) {
		t.Fatalf("applied = %v, want [l2]", applied)
	}
	if !reflect.DeepEqual(ignored, []string{"nope", "r2"}) {
		t.Fatalf("ignored = %v, want [nope r2]", ignored)
	}
	if reasons["r2"] != "gamma must be > 0" || reasons["nope"] != "unknown signal" {
		t.Fatalf("reasons = %v", reasons)
	}
	if sh, _ := set.GetShaper("r2"); sh.Gamma != 1.5 {
		t.Fatalf("r2.Gamma = %v, want 1.5 preserved", sh.Gamma)
	}
	if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.2 {
		t.Fatalf("l2.Deadzone = %v, want 0.2", sh.Deadzone)
	}
}

// TestApplyShapePatch_RejectsUnknownFields covers the reviewer's "silent
// ignore" case at field granularity: a typo or a pre-snake_case spelling
// decodes to all-nil pointers, so the entry must be rejected instead of being
// reported as applied while nothing changed.
func TestApplyShapePatch_RejectsUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"shape":{"l2":{"SlewUp":10}}}`, // old camelCase spelling
		`{"shape":{"l2":{"slewup":10}}}`, // typo
		`{"shape":{"l2":{}}}`,            // no fields at all
	} {
		set := sig.NewSet(map[string]sig.Shaper{"l2": {Deadzone: 0.1}})
		applied, ignored, reasons, err := applyShapePatch(nil, set, json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", raw, err)
		}
		if len(applied) != 0 {
			t.Fatalf("%s: applied = %v, want empty", raw, applied)
		}
		if !reflect.DeepEqual(ignored, []string{"l2"}) || reasons["l2"] != "no recognised fields" {
			t.Fatalf("%s: ignored = %v, reasons = %v", raw, ignored, reasons)
		}
		if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.1 || sh.SlewUp != 0 {
			t.Fatalf("%s: shaper mutated: %+v", raw, sh)
		}
	}
}

// TestProfileAccessorsAreRaceFree exercises the accessor pair used by
// profile.reload against concurrent readers (run with -race to be meaningful).
func TestProfileAccessorsAreRaceFree(t *testing.T) {
	var mu sync.RWMutex
	active := newTestProfile()
	current := func() *profile.Profile {
		mu.RLock()
		defer mu.RUnlock()
		return active
	}
	set := func(p *profile.Profile) {
		mu.Lock()
		active = p
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				next := newTestProfile()
				next.Name = "reloaded"
				set(next)
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = current().TTL()
				_ = current().Extra
			}
		}()
	}
	wg.Wait()
}

// --- profile.reload atomicity ---------------------------------------------

func newReloadProfile(name string, shape sig.Shaper, rules ...profile.Rule) *profile.Profile {
	return &profile.Profile{
		Name:   name,
		Source: profile.Source{Type: "sim"},
		Shape:  map[string]profile.Shape{"l2": {Deadzone: shape.Deadzone, Gamma: shape.Gamma, SlewUp: shape.SlewUp, SlewDown: shape.SlewDown}},
		Rules:  rules,
	}
}

func goodRule(name, value string) profile.Rule {
	return profile.Rule{Name: name, Target: "l2", Effect: "rigid", Value: value,
		Params: map[string]string{"start": "0", "force": "100"}}
}

func newReloadEngine(t *testing.T, p *profile.Profile) (*sig.Set, *rules.Engine) {
	t.Helper()
	if err := p.Validate(); err != nil {
		t.Fatalf("fixture profile invalid: %v", err)
	}
	set := sig.NewSet(map[string]sig.Shaper{"l2": {Deadzone: p.Shape["l2"].Deadzone}})
	eng, err := rules.New(p, set, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	return set, eng
}

func ruleNames(telem []rules.RuleTelemetry) []string {
	out := make([]string, len(telem))
	for i, r := range telem {
		out[i] = r.Name
	}
	return out
}

// TestApplyReload_SuccessSwapsBoth covers the happy path: shaping and rules
// both come from the new profile.
func TestApplyReload_SuccessSwapsBoth(t *testing.T) {
	old := newReloadProfile("old", sig.Shaper{Deadzone: 0.05}, goodRule("old-rule", "100"))
	set, eng := newReloadEngine(t, old)

	next := newReloadProfile("next", sig.Shaper{Deadzone: 0.42}, goodRule("new-rule", "200"))
	if err := applyReload(set, eng, next); err != nil {
		t.Fatalf("applyReload: %v", err)
	}

	if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.42 {
		t.Fatalf("shaper = %v, want 0.42", sh.Deadzone)
	}
	if got := ruleNames(eng.Telemetry(time.Now())); !reflect.DeepEqual(got, []string{"new-rule"}) {
		t.Fatalf("rules = %v, want [new-rule]", got)
	}
}

// TestApplyReload_CompileFailureChangesNothing is the regression test for the
// half-updated state: a rule that fails to compile must leave shaping, rules and
// ids exactly as they were.
func TestApplyReload_CompileFailureChangesNothing(t *testing.T) {
	old := newReloadProfile("old", sig.Shaper{Deadzone: 0.05}, goodRule("old-rule", "100"))
	set, eng := newReloadEngine(t, old)
	before := eng.Telemetry(time.Now())

	broken := goodRule("broken-rule", "100")
	broken.When = "l2 >" // parse error
	next := newReloadProfile("next", sig.Shaper{Deadzone: 0.9}, goodRule("new-rule", "200"), broken)

	if err := applyReload(set, eng, next); err == nil {
		t.Fatal("applyReload succeeded, want compile error")
	}
	if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.05 {
		t.Fatalf("shaper leaked from the failed reload: %v, want 0.05", sh.Deadzone)
	}
	if after := eng.Telemetry(time.Now()); !reflect.DeepEqual(before, after) {
		t.Fatalf("rules changed on failed reload:\nbefore %v\nafter  %v", before, after)
	}
}

// TestApplyReload_ValidateFailureChangesNothing covers a profile that fails
// Validate (duplicate rule names since a18d2aa).
func TestApplyReload_ValidateFailureChangesNothing(t *testing.T) {
	old := newReloadProfile("old", sig.Shaper{Deadzone: 0.05}, goodRule("old-rule", "100"))
	set, eng := newReloadEngine(t, old)
	before := eng.Telemetry(time.Now())

	next := newReloadProfile("dup", sig.Shaper{Deadzone: 0.9},
		goodRule("dup", "100"), goodRule("dup", "200"))

	if err := applyReload(set, eng, next); err == nil {
		t.Fatal("applyReload succeeded, want validate error")
	}
	if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.05 {
		t.Fatalf("shaper leaked from the failed reload: %v", sh.Deadzone)
	}
	if after := eng.Telemetry(time.Now()); !reflect.DeepEqual(before, after) {
		t.Fatalf("rules changed on failed reload:\nbefore %v\nafter  %v", before, after)
	}
}

// TestApplyReload_RecoversAfterFailure proves a failed reload leaves no sticky
// state: the next healthy reload applies normally.
func TestApplyReload_RecoversAfterFailure(t *testing.T) {
	old := newReloadProfile("old", sig.Shaper{Deadzone: 0.05}, goodRule("old-rule", "100"))
	set, eng := newReloadEngine(t, old)

	broken := goodRule("broken-rule", "100")
	broken.When = "l2 >"
	if err := applyReload(set, eng, newReloadProfile("bad", sig.Shaper{Deadzone: 0.9}, broken)); err == nil {
		t.Fatal("expected failure")
	}

	good := newReloadProfile("good", sig.Shaper{Deadzone: 0.33}, goodRule("ok-rule", "150"))
	if err := applyReload(set, eng, good); err != nil {
		t.Fatalf("follow-up reload failed: %v", err)
	}
	if sh, _ := set.GetShaper("l2"); sh.Deadzone != 0.33 {
		t.Fatalf("shaper = %v, want 0.33", sh.Deadzone)
	}
	if got := ruleNames(eng.Telemetry(time.Now())); !reflect.DeepEqual(got, []string{"ok-rule"}) {
		t.Fatalf("rules = %v, want [ok-rule]", got)
	}
}

// TestApplyReload_ConcurrentReads runs reloads (failing and succeeding) while
// readers sample shapers and rule telemetry; meaningful under -race.
func TestApplyReload_ConcurrentReads(t *testing.T) {
	old := newReloadProfile("old", sig.Shaper{Deadzone: 0.05}, goodRule("old-rule", "100"))
	set, eng := newReloadEngine(t, old)

	broken := goodRule("broken-rule", "100")
	broken.When = "l2 >"

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = applyReload(set, eng, newReloadProfile("n", sig.Shaper{Deadzone: 0.2}, goodRule("r", "100")))
				_ = applyReload(set, eng, newReloadProfile("b", sig.Shaper{Deadzone: 0.9}, broken))
			}
		}()
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = set.GetShaper("l2")
				_ = eng.Telemetry(time.Now())
			}
		}()
	}
	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func decodeHealth(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("health JSON is invalid: %v (raw=%s)", err, raw)
	}
	return got
}

// TestHealthDryRunShape checks that the appended device object is present and
// well-formed when no controller is open, while every legacy field survives.
func TestHealthDryRunShape(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	raw, err := buildHealth(start, time.Now(), 3, 125, "fps", "sim", 42, nil, sink.Stats{})
	if err != nil {
		t.Fatalf("buildHealth: %v", err)
	}
	got := decodeHealth(t, raw)

	for _, key := range []string{"uptime_s", "subscribers", "rate_hz", "profile", "source", "jitter_us", "device"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("health payload lost legacy field %q: %s", key, raw)
		}
	}

	dev, ok := got["device"].(map[string]any)
	if !ok {
		t.Fatalf("device is not an object: %T", got["device"])
	}
	if connected, ok := dev["connected"].(bool); !ok || connected {
		t.Fatalf("dry-run device.connected = %v, want false", dev["connected"])
	}
	for _, key := range []string{"path", "transport", "vendor_id", "product_id"} {
		if _, ok := dev[key]; ok {
			t.Fatalf("dry-run device must omit %q, got %v", key, dev[key])
		}
	}
	if dev["write_errors"].(float64) != 0 || dev["reconnects"].(float64) != 0 {
		t.Fatalf("dry-run counters = %v, want 0/0", dev)
	}
}

// TestHealthConnectedDevice covers identity formatting and the cumulative sink
// counters injected from the sink package.
func TestHealthConnectedDevice(t *testing.T) {
	cases := []struct {
		name      string
		transport device.Transport
		want      string
	}{
		{"usb", device.USB, "usb"},
		{"bluetooth", device.BT, "bluetooth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := &deviceStatus{
				connected: true,
				info: device.Info{
					Path:      "/dev/hidraw3",
					Vendor:    0x054c,
					Product:   0x0ce6,
					Transport: tc.transport,
				},
			}
			raw, err := buildHealth(time.Now(), time.Now(), 1, 125, "fps", "trigger", 9,
				status, sink.Stats{WriteErrors: 7, Reconnects: 2})
			if err != nil {
				t.Fatalf("buildHealth: %v", err)
			}
			dev := decodeHealth(t, raw)["device"].(map[string]any)
			want := map[string]any{
				"connected":    true,
				"path":         "/dev/hidraw3",
				"transport":    tc.want,
				"vendor_id":    "054c",
				"product_id":   "0ce6",
				"write_errors": float64(7),
				"reconnects":   float64(2),
			}
			for k, v := range want {
				if dev[k] != v {
					t.Fatalf("device[%q] = %v, want %v", k, dev[k], v)
				}
			}
		})
	}
}

// TestHealthDisconnectedOmitsIdentity makes sure identity is omitted (but
// counters remain queryable) when the sink could not reattach.
func TestHealthDisconnectedOmitsIdentity(t *testing.T) {
	status := &deviceStatus{
		connected: false,
		info:      device.Info{Path: "/dev/hidraw3", Vendor: 0x054c, Product: 0x0ce6, Transport: device.USB},
	}
	raw, err := buildHealth(time.Now(), time.Now(), 0, 125, "fps", "trigger", 0,
		status, sink.Stats{WriteErrors: 4, Reconnects: 0})
	if err != nil {
		t.Fatalf("buildHealth: %v", err)
	}
	dev := decodeHealth(t, raw)["device"].(map[string]any)
	if dev["connected"] != false {
		t.Fatalf("device.connected = %v, want false", dev["connected"])
	}
	if _, ok := dev["path"]; ok {
		t.Fatalf("disconnected device leaked path: %v", dev["path"])
	}
	if dev["write_errors"] != float64(4) {
		t.Fatalf("device.write_errors = %v, want 4", dev["write_errors"])
	}
}

// TestHealthDeviceOmitsUnprobedIdentity covers a forced -hidraw path (or a
// platform that failed to read the descriptor): a zero VID/PID must stay empty
// rather than being formatted as a fabricated "0000".
func TestHealthDeviceOmitsUnprobedIdentity(t *testing.T) {
	d := newHealthDevice(&deviceStatus{connected: true, info: device.Info{Path: "/dev/hidraw9"}}, sink.Stats{})
	if !d.Connected {
		t.Fatal("connected = false, want true")
	}
	if d.VendorID != "" || d.ProductID != "" {
		t.Fatalf("identity = %q/%q, want both empty", d.VendorID, d.ProductID)
	}
}
