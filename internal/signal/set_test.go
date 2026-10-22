package signal_test

import (
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
)

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
