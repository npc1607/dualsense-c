package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/npc1607/dualsense-c/internal/profile"
	sig "github.com/npc1607/dualsense-c/internal/signal"
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
