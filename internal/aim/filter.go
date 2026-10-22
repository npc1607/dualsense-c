package aim

import (
	"math"
	"time"
)

// FilterType defines the jitter reduction algorithm.
type FilterType string

const (
	FilterNone    FilterType = "none"
	FilterEMA     FilterType = "ema"
	FilterOneEuro FilterType = "one_euro"
)

// FilterConfig configures the stick filtering parameters.
type FilterConfig struct {
	Type      FilterType `yaml:"type"`
	MinCutoff float64    `yaml:"min_cutoff"` // Low-speed cutoff in Hz (default ~0.85)
	Beta      float64    `yaml:"beta"`       // Speed coefficient (default ~0.045)
	DCutoff   float64    `yaml:"d_cutoff"`   // Derivative cutoff in Hz (default ~1.0)
}

// OneEuroFilter implements the 1€ filter by Géry Casiez et al.
// It provides high jitter reduction at low speeds while maintaining low latency during fast motions.
type OneEuroFilter struct {
	minCutoff float64
	beta      float64
	dCutoff   float64
	prevX     float64
	prevDX    float64
	hasPrev   bool
}

// NewOneEuroFilter creates a single-axis One Euro filter.
func NewOneEuroFilter(minCutoff, beta, dCutoff float64) *OneEuroFilter {
	if minCutoff <= 0 {
		minCutoff = 0.85
	}
	if beta < 0 {
		beta = 0.045
	}
	if dCutoff <= 0 {
		dCutoff = 1.0
	}
	return &OneEuroFilter{
		minCutoff: minCutoff,
		beta:      beta,
		dCutoff:   dCutoff,
	}
}

func alpha(dt, cutoff float64) float64 {
	tau := 1.0 / (2.0 * math.Pi * cutoff)
	te := dt
	return 1.0 / (1.0 + tau/te)
}

// Filter processes one scalar measurement with time step dt (in seconds).
func (f *OneEuroFilter) Filter(x float64, dt float64) float64 {
	if dt <= 0 || dt > 0.1 {
		dt = 0.008 // default to 125Hz interval if invalid or anomalous
	}
	if !f.hasPrev {
		f.prevX = x
		f.prevDX = 0
		f.hasPrev = true
		return x
	}

	// Filter the derivative
	dx := (x - f.prevX) / dt
	aD := alpha(dt, f.dCutoff)
	hatDX := aD*dx + (1.0-aD)*f.prevDX

	// Adaptive cutoff frequency
	cutoff := f.minCutoff + f.beta*math.Abs(hatDX)
	aX := alpha(dt, cutoff)
	hatX := aX*x + (1.0-aX)*f.prevX

	f.prevX = hatX
	f.prevDX = hatDX
	return hatX
}

// Reset clears historical state.
func (f *OneEuroFilter) Reset() {
	f.prevX = 0
	f.prevDX = 0
	f.hasPrev = false
}

// StickFilter handles 2D stick filtering.
type StickFilter struct {
	cfg    FilterConfig
	filtX  *OneEuroFilter
	filtY  *OneEuroFilter
	emaX   float64
	emaY   float64
	hasEMA bool
}

// NewStickFilter creates a StickFilter based on config.
func NewStickFilter(cfg FilterConfig) *StickFilter {
	if cfg.Type == "" {
		cfg.Type = FilterNone
	}
	return &StickFilter{
		cfg:   cfg,
		filtX: NewOneEuroFilter(cfg.MinCutoff, cfg.Beta, cfg.DCutoff),
		filtY: NewOneEuroFilter(cfg.MinCutoff, cfg.Beta, cfg.DCutoff),
	}
}

// Filter filters (x, y) stick inputs using the configured filter.
func (sf *StickFilter) Filter(x, y float32, dt time.Duration) (float32, float32) {
	switch sf.cfg.Type {
	case FilterOneEuro:
		dtSec := dt.Seconds()
		if dtSec <= 0 || dtSec > 0.1 {
			dtSec = 0.008
		}
		fx := sf.filtX.Filter(float64(x), dtSec)
		fy := sf.filtY.Filter(float64(y), dtSec)
		return float32(fx), float32(fy)

	case FilterEMA:
		const alphaEMA = 0.35
		if !sf.hasEMA {
			sf.emaX = float64(x)
			sf.emaY = float64(y)
			sf.hasEMA = true
			return x, y
		}
		sf.emaX = alphaEMA*float64(x) + (1.0-alphaEMA)*sf.emaX
		sf.emaY = alphaEMA*float64(y) + (1.0-alphaEMA)*sf.emaY
		return float32(sf.emaX), float32(sf.emaY)

	case FilterNone:
		fallthrough
	default:
		return x, y
	}
}

// Reset resets filter internal state.
func (sf *StickFilter) Reset() {
	sf.filtX.Reset()
	sf.filtY.Reset()
	sf.hasEMA = false
}
