package aim

import (
	"math"
)

// CurveType defines the mathematical transfer function applied to stick magnitude.
type CurveType string

const (
	CurveLinear   CurveType = "linear"
	CurvePower    CurveType = "power"
	CurveSigmoid  CurveType = "sigmoid"
	CurveDualZone CurveType = "dual_zone"
)

// CurveConfig parameters for stick response curve shaping.
type CurveConfig struct {
	Type          CurveType `yaml:"type"`
	Exponent      float64   `yaml:"exponent"`       // Power gamma or Sigmoid slope k
	MidPoint      float64   `yaml:"mid_point"`      // Sigmoid r0 or DualZone r_split
	InnerDeadzone float64   `yaml:"inner_deadzone"` // Radial deadzone threshold [0..1)
	OuterBoost    float64   `yaml:"outer_boost"`    // Speed multiplier near rim (r >= 0.95)
}

// CurveProcessor applies radial deadzone, non-linear curves, and outer boost
// while strictly preserving stick angle.
type CurveProcessor struct {
	cfg CurveConfig
}

// NewCurve creates a CurveProcessor with validated defaults.
func NewCurve(cfg CurveConfig) *CurveProcessor {
	if cfg.Type == "" {
		cfg.Type = CurveLinear
	}
	if cfg.Type == CurvePower && cfg.Exponent <= 0 {
		cfg.Exponent = 2.0
	}
	if cfg.Type == CurveSigmoid {
		if cfg.Exponent <= 0 {
			cfg.Exponent = 6.5
		}
		if cfg.MidPoint <= 0 || cfg.MidPoint >= 1.0 {
			cfg.MidPoint = 0.52
		}
	}
	if cfg.Type == CurveDualZone {
		if cfg.MidPoint <= 0 || cfg.MidPoint >= 1.0 {
			cfg.MidPoint = 0.85
		}
	}
	if cfg.InnerDeadzone < 0 {
		cfg.InnerDeadzone = 0
	}
	if cfg.InnerDeadzone >= 1.0 {
		cfg.InnerDeadzone = 0.99
	}
	return &CurveProcessor{cfg: cfg}
}

// Transform evaluates the non-linear transfer function on a normalized [0, 1] radial scalar.
func (c *CurveProcessor) Transform(rNorm float64) float64 {
	if rNorm <= 0 {
		return 0
	}
	if rNorm >= 1.0 {
		return 1.0
	}

	var rCurved float64
	switch c.cfg.Type {
	case CurvePower:
		gamma := c.cfg.Exponent
		if gamma <= 0 {
			gamma = 2.0
		}
		rCurved = math.Pow(rNorm, gamma)

	case CurveSigmoid:
		k := c.cfg.Exponent
		if k <= 0 {
			k = 6.5
		}
		r0 := c.cfg.MidPoint
		if r0 <= 0 || r0 >= 1.0 {
			r0 = 0.52
		}
		f := func(u float64) float64 {
			return 1.0 / (1.0 + math.Exp(-k*(u-r0)))
		}
		f0 := f(0)
		f1 := f(1)
		denom := f1 - f0
		if denom > 1e-9 {
			rCurved = (f(rNorm) - f0) / denom
		} else {
			rCurved = rNorm
		}

	case CurveDualZone:
		rSplit := c.cfg.MidPoint
		if rSplit <= 0 || rSplit >= 1.0 {
			rSplit = 0.85
		}
		alpha := 0.35
		if c.cfg.Exponent > 0 && c.cfg.Exponent < 1.0 {
			alpha = c.cfg.Exponent
		}
		if rNorm < rSplit {
			rCurved = alpha * (rNorm / rSplit)
		} else {
			rCurved = alpha + (1.0-alpha)*((rNorm-rSplit)/(1.0-rSplit))
		}

	case CurveLinear:
		fallthrough
	default:
		rCurved = rNorm
	}

	// Outer Boost acceleration near rim (rNorm >= 0.95)
	if rNorm > 0.95 && c.cfg.OuterBoost > 1.0 {
		boost := 1.0 + (c.cfg.OuterBoost-1.0)*((rNorm-0.95)/0.05)
		rCurved = math.Min(1.0, rCurved*boost)
	}

	if rCurved < 0 {
		rCurved = 0
	}
	if rCurved > 1.0 {
		rCurved = 1.0
	}
	return rCurved
}

// TransformStick takes raw stick axes (-1..1), evaluates deadzone and curve,
// and returns the reshaped stick axes (-1..1) preserving vector direction.
func (c *CurveProcessor) TransformStick(x, y float32) (outX, outY float32) {
	fx := float64(x)
	fy := float64(y)
	r := math.Sqrt(fx*fx + fy*fy)

	if r <= c.cfg.InnerDeadzone || r < 1e-7 {
		return 0, 0
	}

	// Normalize r into [0, 1] beyond deadzone
	rNorm := (r - c.cfg.InnerDeadzone) / (1.0 - c.cfg.InnerDeadzone)
	if rNorm < 0 {
		rNorm = 0
	}
	if rNorm > 1.0 {
		rNorm = 1.0
	}

	rCurved := c.Transform(rNorm)

	scale := rCurved / r
	ox := fx * scale
	oy := fy * scale

	if ox < -1.0 {
		ox = -1.0
	} else if ox > 1.0 {
		ox = 1.0
	}
	if oy < -1.0 {
		oy = -1.0
	} else if oy > 1.0 {
		oy = 1.0
	}

	return float32(ox), float32(oy)
}
