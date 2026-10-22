package signal

import (
	"math"
	"sync"
	"time"
)

// Frame is one update from a Source. At is the capture timestamp used for
// staleness checks, so a source that batches data can still be accurate.
type Frame struct {
	Values map[string]float32
	At     time.Time
}

// Shaper smooths a raw signal before rules see it. Zero value = pass-through.
//
//	SlewUp/SlewDown are per-second limits (e.g. 8 = 8 units/second).
//	Deadzone zeroes small magnitudes (useful for stick/trigger noise).
//	Gamma < 1 makes small inputs bigger, > 1 compresses them.
type Shaper struct {
	Deadzone float64 `yaml:"deadzone"`
	Gamma    float64 `yaml:"gamma"`
	SlewUp   float64 `yaml:"slew_up"`
	SlewDown float64 `yaml:"slew_down"`
}

func (s Shaper) apply(prev, raw float64, dt time.Duration) float64 {
	v := raw
	if s.Deadzone > 0 && math.Abs(v) < s.Deadzone {
		v = 0
	}
	if s.Gamma > 0 && s.Gamma != 1 {
		v = math.Copysign(math.Pow(math.Abs(v), s.Gamma), v)
	}
	if dt > 0 {
		limit := s.SlewUp
		if math.Abs(v) < math.Abs(prev) {
			limit = s.SlewDown
		}
		if limit > 0 {
			maxDelta := limit * dt.Seconds()
			if d := v - prev; math.Abs(d) > maxDelta {
				v = prev + math.Copysign(maxDelta, d)
			}
		}
	}
	return v
}

type entry struct {
	raw    float64
	shaped float64
	at     time.Time
}

// Set is the shared, concurrency-safe store of current signal values.
// One source goroutine writes; the sink goroutine reads once per output tick.
type Set struct {
	mu      sync.RWMutex
	vals    map[string]*entry
	shapers map[string]Shaper
}

// NewSet returns an empty set. shapers may be nil.
func NewSet(shapers map[string]Shaper) *Set {
	return &Set{vals: make(map[string]*entry, 32), shapers: shapers}
}

// Apply merges a source frame, running shaping against the previous value.
func (s *Set) Apply(f Frame) {
	if len(f.Values) == 0 {
		return
	}
	at := f.At
	if at.IsZero() {
		at = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, raw := range f.Values {
		v := float64(raw)
		e := s.vals[name]
		if e == nil {
			e = &entry{at: at}
			s.vals[name] = e
		}
		dt := at.Sub(e.at)
		if dt < 0 {
			dt = 0
		}
		e.shaped = s.shapers[name].apply(e.shaped, v, dt)
		e.raw = v
		e.at = at
	}
}

// Snapshot returns the shaped values that are newer than ttl, plus the set of
// names considered fresh. Expired signals are omitted entirely, so has()
// returns 0 and rules can fall back.
func (s *Set) Snapshot(now time.Time, ttl time.Duration) (map[string]float64, map[string]bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]float64, len(s.vals))
	fresh := make(map[string]bool, len(s.vals))
	for name, e := range s.vals {
		if ttl > 0 && now.Sub(e.at) > ttl {
			continue
		}
		out[name] = e.shaped
		fresh[name] = true
	}
	return out, fresh
}

// Fresh reports whether any signal arrived within ttl.
func (s *Set) Fresh(now time.Time, ttl time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.vals {
		if now.Sub(e.at) <= ttl {
			return true
		}
	}
	return false
}

// SignalTelemetry carries one signal's raw and shaped state plus staleness metadata.
type SignalTelemetry struct {
	ID     uint8
	Name   string
	Raw    float64
	Shaped float64
	AgeMS  uint16
	Fresh  bool
}

// Telemetry returns a non-destructive snapshot of all standard signals with age and freshness.
func (s *Set) Telemetry(now time.Time, ttl time.Duration) []SignalTelemetry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]SignalTelemetry, len(Standard))
	for i, name := range Standard {
		st := SignalTelemetry{
			ID:    uint8(i),
			Name:  name,
			AgeMS: 0xFFFF,
			Fresh: false,
		}
		if e := s.vals[name]; e != nil {
			st.Raw = e.raw
			st.Shaped = e.shaped
			age := now.Sub(e.at)
			if age < 0 {
				st.AgeMS = 0
			} else if ms := age.Milliseconds(); ms > 0xFFFF {
				st.AgeMS = 0xFFFF
			} else {
				st.AgeMS = uint16(ms)
			}
			st.Fresh = (ttl <= 0 || age <= ttl)
		}
		out[i] = st
	}
	return out
}

// SetShapers replaces the shapers map atomically without discarding existing signal history.
func (s *Set) SetShapers(shapers map[string]Shaper) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shapers = shapers
}
