package audio

import (
	"io"
	"log"
	"math"
	"testing"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/source"
)

func mustSource(t *testing.T, params map[string]any) profile.Source {
	t.Helper()
	return profile.Source{Type: "audio", Params: params}
}

func testDeps() source.Deps {
	return source.Deps{Logger: log.New(io.Discard, "", 0)}
}

// tone returns n samples of a sine at hz with the given peak amplitude.
func tone(hz, amp float64, rate int, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return out
}

// measureOnce feeds a long tone through the filters and returns the settled
// band energies.
func measureOnce(s *Source, samples []float32) (bass, mid, loud float64) {
	const block = 256 // matches the runtime block size
	for i := 0; i+block <= len(samples); i += block {
		bass, mid, loud = s.measure(samples[i : i+block])
	}
	return bass, mid, loud
}

func newTestSource(t *testing.T) *Source {
	t.Helper()
	src, err := New(mustSource(t, map[string]any{"target": "test"}), testDeps())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return src.(*Source)
}

// A 50 Hz rumble must land in `bass` and stay out of `mid`.
func TestBassBandSeparatesLowFromMid(t *testing.T) {
	s := newTestSource(t)
	low := tone(50, 0.3, s.rate, s.rate)
	// Run twice: the first pass primes the filter and envelope history.
	measureOnce(s, low)
	bass, mid, _ := measureOnce(s, low)

	if bass < 0.3 {
		t.Errorf("bass = %.3f for 50 Hz tone, want > 0.3", bass)
	}
	if mid > 0.1 {
		t.Errorf("mid = %.3f for 50 Hz tone, want < 0.1 (band leak)", mid)
	}

	other := newTestSource(t)
	high := tone(1000, 0.3, other.rate, other.rate)
	measureOnce(other, high)
	bass2, mid2, loud2 := measureOnce(other, high)
	if bass2 > 0.05 {
		t.Errorf("bass = %.3f for 1 kHz tone, want ~0", bass2)
	}
	// 36 dB/oct still lets a little through, and the sqrt normalisation is
	// deliberately sensitive at the bottom of the range, so allow some leak.
	if mid2 > 0.15 {
		t.Errorf("mid = %.3f for 1 kHz tone, want < 0.15 (above the mid band)", mid2)
	}
	if loud2 < 0.3 {
		t.Errorf("loudness = %.3f for 1 kHz tone, want > 0.3 (broadband)", loud2)
	}
}

// Silence must stay silent, otherwise the profile would rumble forever.
func TestSilenceStaysSilent(t *testing.T) {
	s := newTestSource(t)
	silent := make([]float32, s.rate)
	bass, mid, loud := measureOnce(s, silent)
	if bass != 0 || mid != 0 || loud != 0 {
		t.Errorf("silence produced bass=%v mid=%v loud=%v", bass, mid, loud)
	}
}

// Louder input must produce a larger envelope (monotonic response).
func TestEnvelopeIsMonotonicInAmplitude(t *testing.T) {
	quiet := newTestSource(t)
	quiet.measure(silentBlock())
	qb, _, _ := measureOnce(quiet, tone(60, 0.05, quiet.rate, quiet.rate))

	plain := newTestSource(t)
	plain.measure(silentBlock())
	pb, _, _ := measureOnce(plain, tone(60, 0.5, plain.rate, plain.rate))

	if qb == 0 || pb == 0 {
		t.Fatalf("expected non-zero envelopes, got quiet=%v loud=%v", qb, pb)
	}
	if pb <= qb {
		t.Errorf("loud envelope %.3f should exceed quiet %.3f", pb, qb)
	}
	if pb > 1 || qb > 1 {
		t.Errorf("envelopes must be clamped to 1: quiet=%v loud=%v", qb, pb)
	}
}

func silentBlock() []float32 { return make([]float32, 1024) }

func TestParseBand(t *testing.T) {
	lo, hi := parseBand("30,200", 20, 120)
	if lo != 30 || hi != 200 {
		t.Errorf("parseBand = %v,%v", lo, hi)
	}
	lo, hi = parseBand("", 20, 120)
	if lo != 20 || hi != 120 {
		t.Errorf("default parseBand = %v,%v", lo, hi)
	}
	lo, hi = parseBand("200,50", 20, 120) // reversed -> repaired
	if hi <= lo {
		t.Errorf("reversed band not repaired: %v,%v", lo, hi)
	}
}

// The transient detector must fire on a burst after silence and stay quiet on a
// steady tone, even when the tone is loud (game music saturates loudness).
func TestOnsetDetectsTransientNotLevel(t *testing.T) {
	s := newTestSource(t)
	const block = 256

	// Steady loud tone for a while: onset should settle to ~0.
	steady := tone(60, 0.5, s.rate, s.rate)
	var onset float32
	for i := 0; i+block <= len(steady); i += block {
		s.measure(steady[i : i+block])
		onset = s.lastOnset
	}
	if onset > 0.15 {
		t.Errorf("steady tone onset = %.3f, want < 0.15 (no transient)", onset)
	}

	// Silence, then a burst: onset must spike.
	silent := make([]float32, block*60) // ~1s: let the bed tracker settle
	for i := 0; i < len(silent); i += block {
		s.measure(silent[i : i+block])
	}
	burst := tone(60, 0.5, s.rate, block*2)
	var peak float32
	for i := 0; i+block <= len(burst); i += block {
		s.measure(burst[i : i+block])
		if s.lastOnset > peak {
			peak = s.lastOnset
		}
	}
	if peak < 0.4 {
		t.Errorf("burst onset = %.3f, want > 0.4", peak)
	}
}
