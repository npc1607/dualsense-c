package aim

import (
	"sync"
	"time"
)

// InputFrame captures all real-time controller inputs needed for aim processing.
type InputFrame struct {
	RX, RY      float32
	GyroYaw     float32
	GyroPitch   float32
	GyroRoll    float32
	L2, R2      float32
	TouchActive bool
	At          time.Time
}

// Processor coordinates the full aim assist pipeline:
// raw stick -> curve -> jitter filter -> gyro blending -> anti-recoil -> clamping.
type Processor struct {
	mu     sync.Mutex
	curve  *CurveProcessor
	filter *StickFilter
	gyro   *GyroMapper
	recoil *AntiRecoil
	lastAt time.Time
}

// NewProcessor constructs an initialized aim Processor.
func NewProcessor(curveCfg CurveConfig, filterCfg FilterConfig, gyroCfg GyroConfig, recoilCfg AntiRecoilConfig) *Processor {
	return &Processor{
		curve:  NewCurve(curveCfg),
		filter: NewStickFilter(filterCfg),
		gyro:   NewGyroMapper(gyroCfg),
		recoil: NewAntiRecoil(recoilCfg),
	}
}

// Process passes an input frame through the composite aim assist pipeline.
func (p *Processor) Process(in InputFrame) (outRX, outRY float32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := in.At
	if now.IsZero() {
		now = time.Now()
	}

	var dt time.Duration
	if p.lastAt.IsZero() {
		dt = 8 * time.Millisecond
	} else {
		dt = now.Sub(p.lastAt)
		if dt <= 0 || dt > 100*time.Millisecond {
			dt = 8 * time.Millisecond
		}
	}
	p.lastAt = now

	// 1. Response Curve
	cx, cy := p.curve.TransformStick(in.RX, in.RY)

	// 2. Adaptive Jitter Filter
	fx, fy := p.filter.Filter(cx, cy, dt)

	// 3. Gyro Blend
	deltaGX, deltaGY := p.gyro.Process(in.GyroYaw, in.GyroPitch, in.GyroRoll, in.L2, in.TouchActive)
	combinedX := fx + deltaGX
	combinedY := fy + deltaGY

	// 4. Anti-Recoil pulldown
	deltaRecoil := p.recoil.Process(in.R2, now)
	combinedY += deltaRecoil

	// 5. Final Clamping
	if combinedX < -1.0 {
		combinedX = -1.0
	} else if combinedX > 1.0 {
		combinedX = 1.0
	}
	if combinedY < -1.0 {
		combinedY = -1.0
	} else if combinedY > 1.0 {
		combinedY = 1.0
	}

	return combinedX, combinedY
}

// Reset clears historical state in all sub-processors.
func (p *Processor) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.filter.Reset()
	p.recoil.Reset()
	p.lastAt = time.Time{}
}
