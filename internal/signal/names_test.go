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

// The bridge wire format depends on ids matching the vocabulary order; pin a
// few so an accidental reorder is caught immediately.
func TestIDPinned(t *testing.T) {
	if id, _ := ID("throttle"); id != 0 {
		t.Errorf("throttle id = %d, want 0", id)
	}
	if id, _ := ID("brake"); id != 1 {
		t.Errorf("brake id = %d, want 1", id)
	}
}
