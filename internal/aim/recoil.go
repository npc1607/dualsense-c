package aim

import (
	"math"
	"math/rand"
	"time"
)

// AntiRecoilConfig defines autonomous pull-down recoil compensation.
type AntiRecoilConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Trigger      string        `yaml:"trigger"`       // e.g. "r2_full"
	StartDelay   time.Duration `yaml:"start_delay"`   // Delay before compensation kicks in
	BurstForce   float64       `yaml:"burst_force"`   // Peak burst pulldown force (0..32768)
	BurstDecay   float64       `yaml:"burst_decay"`   // Exponential decay rate
	SustainForce float64       `yaml:"sustain_force"` // Steady-state pulldown force
	JitterNoise  float64       `yaml:"jitter_noise"`  // Gaussian noise standard deviation
}

// AntiRecoil computes vertical stick compensation over continuous trigger pull.
type AntiRecoil struct {
	cfg       AntiRecoilConfig
	active    bool
	startTime time.Time
	rng       *rand.Rand
}

// NewAntiRecoil creates an AntiRecoil engine.
func NewAntiRecoil(cfg AntiRecoilConfig) *AntiRecoil {
	return &AntiRecoil{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Process evaluates trigger state and returns vertical stick offset (delta RY).
// On standard gamepads, down is positive RY.
func (ar *AntiRecoil) Process(r2 float32, now time.Time) float32 {
	if !ar.cfg.Enabled {
		return 0
	}

	// Trigger condition: R2 fully depressed (>= 0.85)
	if r2 < 0.85 {
		ar.active = false
		return 0
	}

	if !ar.active {
		ar.active = true
		ar.startTime = now
		return 0
	}

	elapsed := now.Sub(ar.startTime)
	if elapsed < ar.cfg.StartDelay {
		return 0
	}

	tEffMS := float64(elapsed-ar.cfg.StartDelay) / float64(time.Millisecond)

	decay := ar.cfg.BurstDecay
	if decay <= 0 {
		decay = 0.015
	}

	force := ar.cfg.BurstForce*math.Exp(-decay*tEffMS) + ar.cfg.SustainForce

	if ar.cfg.JitterNoise > 0 {
		jitter := ar.rng.NormFloat64() * ar.cfg.JitterNoise
		force += jitter
	}

	if force < 0 {
		force = 0
	}

	// Convert 0..32768 force scale to normalized stick offset [0..1]
	deltaRY := force / 32768.0
	if deltaRY > 1.0 {
		deltaRY = 1.0
	}

	return float32(deltaRY)
}

// Reset clears recoil timing state.
func (ar *AntiRecoil) Reset() {
	ar.active = false
}
