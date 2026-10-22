package radio

import (
	"sync"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
)

// TelemetryWindow maintains a continuous sliding window of incoming signal updates
// and provides thread-safe temporal calculus and persistence queries.
type TelemetryWindow struct {
	mu       sync.RWMutex
	frames   []signal.Frame
	duration time.Duration
}

// NewTelemetryWindow creates a TelemetryWindow with the given retention duration.
// If duration <= 0, a default of 5 seconds is used.
func NewTelemetryWindow(duration time.Duration) *TelemetryWindow {
	if duration <= 0 {
		duration = 5 * time.Second
	}
	return &TelemetryWindow{
		duration: duration,
	}
}

// Push appends a telemetry frame and evicts any frames older than the window duration.
func (w *TelemetryWindow) Push(f signal.Frame) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if f.At.IsZero() {
		f.At = time.Now()
	}

	w.frames = append(w.frames, f)
	cutoff := f.At.Add(-w.duration)

	idx := 0
	for idx < len(w.frames) && w.frames[idx].At.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		copy(w.frames, w.frames[idx:])
		clear(w.frames[len(w.frames)-idx:])
		w.frames = w.frames[:len(w.frames)-idx]
	}
}

// Latest returns the most recent frame in the window, if available.
func (w *TelemetryWindow) Latest() (signal.Frame, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	n := len(w.frames)
	if n == 0 {
		return signal.Frame{}, false
	}
	return w.frames[n-1], true
}

// LatestVal returns the value of signal name in the most recent frame (or 0 if missing).
func (w *TelemetryWindow) LatestVal(name string) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	n := len(w.frames)
	if n == 0 {
		return 0
	}
	return w.frames[n-1].Values[name]
}

// DurationAbove calculates the contiguous duration backwards from the latest frame
// where signal name has been sustained at or above threshold.
func (w *TelemetryWindow) DurationAbove(name string, threshold float32) time.Duration {
	return w.DurationSustained(func(vals map[string]float32) bool {
		return vals[name] >= threshold
	})
}

// DurationSustained calculates the contiguous duration backwards from the latest frame
// where predicate remains true. Breaks on the first frame failing predicate.
func (w *TelemetryWindow) DurationSustained(predicate func(vals map[string]float32) bool) time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()

	n := len(w.frames)
	if n == 0 {
		return 0
	}
	if !predicate(w.frames[n-1].Values) {
		return 0
	}

	earliest := n - 1
	for i := n - 2; i >= 0; i-- {
		if !predicate(w.frames[i].Values) {
			break
		}
		earliest = i
	}
	return w.frames[n-1].At.Sub(w.frames[earliest].At)
}

// Average calculates the average value of signal name over the past duration d.
func (w *TelemetryWindow) Average(name string, d time.Duration) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	n := len(w.frames)
	if n == 0 {
		return 0
	}

	latestTime := w.frames[n-1].At
	cutoff := latestTime.Add(-d)
	var sum float32
	count := 0
	for i := n - 1; i >= 0; i-- {
		if w.frames[i].At.Before(cutoff) {
			break
		}
		sum += w.frames[i].Values[name]
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float32(count)
}

// Derivative calculates (latest - previous) / dt over the last two frames.
func (w *TelemetryWindow) Derivative(name string) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	n := len(w.frames)
	if n < 2 {
		return 0
	}
	latest := w.frames[n-1]
	prev := w.frames[n-2]
	dt := latest.At.Sub(prev.At).Seconds()
	if dt < 0.001 {
		return 0
	}
	return (latest.Values[name] - prev.Values[name]) / float32(dt)
}

// Count returns the number of frames currently stored in the window.
func (w *TelemetryWindow) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.frames)
}

// Clear removes all frames from the window.
func (w *TelemetryWindow) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	clear(w.frames)
	w.frames = nil
}
