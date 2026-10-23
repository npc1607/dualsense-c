package signal_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
	"gopkg.in/yaml.v3"
)

func TestShaper_JSONAndYAMLTags(t *testing.T) {
	var fromJSON signal.Shaper
	if err := json.Unmarshal([]byte(`{"deadzone":0.1,"gamma":1.2,"slew_up":10,"slew_down":20}`), &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	want := signal.Shaper{Deadzone: 0.1, Gamma: 1.2, SlewUp: 10, SlewDown: 20}
	if fromJSON != want {
		t.Fatalf("json tags not honoured: got %+v, want %+v", fromJSON, want)
	}

	var fromYAML signal.Shaper
	if err := yaml.Unmarshal([]byte("deadzone: 0.1\ngamma: 1.2\nslew_up: 10\nslew_down: 20\n"), &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if fromYAML != want {
		t.Fatalf("yaml tags not honoured: got %+v, want %+v", fromYAML, want)
	}
}

func TestSet_MergeShapersPreservesOthers(t *testing.T) {
	s := signal.NewSet(map[string]signal.Shaper{
		"l2": {Deadzone: 0.1},
		"r2": {Deadzone: 0.2},
	})

	applied := s.MergeShapers(map[string]signal.Shaper{
		"l2": {Deadzone: 0.3},
	})
	if !reflect.DeepEqual(applied, []string{"l2"}) {
		t.Fatalf("applied = %v, want [l2]", applied)
	}

	if sh, ok := s.GetShaper("l2"); !ok || sh.Deadzone != 0.3 {
		t.Fatalf("l2 shaper = %+v (ok=%v), want deadzone 0.3", sh, ok)
	}
	if sh, ok := s.GetShaper("r2"); !ok || sh.Deadzone != 0.2 {
		t.Fatalf("r2 shaper = %+v (ok=%v), want deadzone 0.2 preserved", sh, ok)
	}

	// A set built without shapers must lazily allocate on merge, and an empty
	// patch must still return a non-nil slice so JSON renders [].
	empty := signal.NewSet(nil)
	if got := empty.MergeShapers(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty patch applied = %#v, want non-nil empty slice", got)
	}
	if got := empty.MergeShapers(map[string]signal.Shaper{"l2": {Gamma: 1.2}}); !reflect.DeepEqual(got, []string{"l2"}) {
		t.Fatalf("lazy merge applied = %v, want [l2]", got)
	}
}

func TestSet_TelemetryAndSetShapers(t *testing.T) {
	shapers := map[string]signal.Shaper{
		"throttle": {Deadzone: 0.1},
	}
	s := signal.NewSet(shapers)

	t0 := time.Now()
	// Before any frames arrive
	telem := s.Telemetry(t0, 400*time.Millisecond)
	if len(telem) != len(signal.Standard) {
		t.Fatalf("expected %d telemetry items, got %d", len(signal.Standard), len(telem))
	}
	if telem[0].Fresh {
		t.Fatalf("expected absent signal to not be fresh")
	}

	// Apply frame
	s.Apply(signal.Frame{
		Values: map[string]float32{
			"throttle": 0.5,
		},
		At: t0,
	})

	telem = s.Telemetry(t0, 400*time.Millisecond)
	thID, ok := signal.ID("throttle")
	if !ok {
		t.Fatal("throttle not found in Standard")
	}
	if !telem[thID].Fresh {
		t.Fatalf("expected throttle to be fresh")
	}
	if telem[thID].Raw != 0.5 {
		t.Fatalf("expected raw 0.5, got %f", telem[thID].Raw)
	}

	// Change shapers
	newShapers := map[string]signal.Shaper{
		"throttle": {Deadzone: 0.2},
	}
	s.SetShapers(newShapers)

	// Existing history and freshness should be preserved
	telemAfter := s.Telemetry(t0, 400*time.Millisecond)
	if !telemAfter[thID].Fresh {
		t.Fatalf("expected throttle to still be fresh after SetShapers")
	}
	if telemAfter[thID].Raw != 0.5 {
		t.Fatalf("expected raw 0.5 after SetShapers, got %f", telemAfter[thID].Raw)
	}
}
