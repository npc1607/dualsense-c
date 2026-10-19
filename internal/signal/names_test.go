package signal

import "testing"

func TestIDsAreStableAndUnique(t *testing.T) {
	if len(Standard) > 255 {
		t.Fatalf("vocabulary too large for the 1-byte wire id: %d", len(Standard))
	}
	seen := make(map[string]bool, len(Standard))
	for _, n := range Standard {
		if seen[n] {
			t.Errorf("duplicate signal name %q", n)
		}
		seen[n] = true
		id, ok := ID(n)
		if !ok {
			t.Fatalf("ID(%q) failed", n)
		}
		if got := Name(id); got != n {
			t.Errorf("Name(%d) = %q, want %q", id, got, n)
		}
	}
	if _, ok := ID("nope"); ok {
		t.Error("unknown name got an id")
	}
	if got := Name(255); got != "" {
		t.Errorf("Name(255) = %q, want empty", got)
	}
}

// The bridge wire format depends on ids matching the vocabulary order, and a
// deployed ds5bridge.exe keeps sending the ids it was built with. Pin a few so
// an accidental reorder (or an insert in the middle) is caught immediately.
func TestIDPinned(t *testing.T) {
	pinned := map[string]uint8{
		"throttle": 0,
		"brake":    1,
		"slip":     7,
		"custom0":  42,
	}
	for name, want := range pinned {
		if id, ok := ID(name); !ok || id != want {
			t.Errorf("%s id = %d (ok=%v), want %d -- append new names, never insert", name, id, ok, want)
		}
	}
}
