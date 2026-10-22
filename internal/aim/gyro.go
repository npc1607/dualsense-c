package aim

import (
	"math"
)

// GyroTriggerType specifies the condition for activating gyro aiming.
type GyroTriggerType string

const (
	GyroTriggerL2Hold GyroTriggerType = "l2_hold"
	GyroTriggerTouch  GyroTriggerType = "touch"
	GyroTriggerAlways GyroTriggerType = "always"
)

// GyroConfig defines IMU gyro blending settings.
type GyroConfig struct {
	Enabled          bool            `yaml:"enabled"`
	Trigger          GyroTriggerType `yaml:"trigger"`
	TriggerThreshold float32         `yaml:"trigger_threshold"`
	NaturalAngleDeg  float64         `yaml:"natural_angle_deg"`
	DeadzoneDPS      float64         `yaml:"deadzone_dps"`
	SensYaw          float64         `yaml:"sens_yaw"`
	SensPitch        float64         `yaml:"sens_pitch"`
	InvertPitch      bool            `yaml:"invert_pitch"`
}

// GyroMapper projects 3-axis angular velocity into stick deltas.
type GyroMapper struct {
	cfg GyroConfig
}

// NewGyroMapper creates a GyroMapper with validated defaults.
func NewGyroMapper(cfg GyroConfig) *GyroMapper {
	if cfg.NaturalAngleDeg <= 0 {
		cfg.NaturalAngleDeg = 35.0
	}
	if cfg.SensYaw == 0 {
		cfg.SensYaw = 1.85
	}
	if cfg.SensPitch == 0 {
		cfg.SensPitch = 1.35
	}
	if cfg.Trigger == "" {
		cfg.Trigger = GyroTriggerL2Hold
	}
	if cfg.TriggerThreshold <= 0 {
		cfg.TriggerThreshold = 0.15
	}
	return &GyroMapper{cfg: cfg}
}

// IsActive checks if gyro aiming condition is currently met.
func (g *GyroMapper) IsActive(l2 float32, touchActive bool) bool {
	if !g.cfg.Enabled {
		return false
	}
	switch g.cfg.Trigger {
	case GyroTriggerL2Hold:
		return l2 >= g.cfg.TriggerThreshold
	case GyroTriggerTouch:
		return touchActive
	case GyroTriggerAlways:
		return true
	default:
		return l2 >= g.cfg.TriggerThreshold
	}
}

// Process transforms raw gyro angular rates (-1..1 representing full scale ~2000 DPS)
// into stick deflection adjustments.
func (g *GyroMapper) Process(gyroYaw, gyroPitch, gyroRoll float32, l2 float32, touchActive bool) (deltaX, deltaY float32) {
	if !g.IsActive(l2, touchActive) {
		return 0, 0
	}

	const dpsFullScale = 2000.0
	dpsYaw := float64(gyroYaw) * dpsFullScale
	dpsPitch := float64(gyroPitch) * dpsFullScale
	omega := math.Sqrt(dpsYaw*dpsYaw + dpsPitch*dpsPitch)

	if g.cfg.DeadzoneDPS > 0 && omega < g.cfg.DeadzoneDPS {
		return 0, 0
	}

	rad := g.cfg.NaturalAngleDeg * math.Pi / 180.0
	cosTheta := math.Cos(rad)
	sinTheta := math.Sin(rad)

	// Rotate yaw by hand tilt angle: yaw_eff = yaw * cos(theta) - roll * sin(theta)
	effYaw := float64(gyroYaw)*cosTheta - float64(gyroRoll)*sinTheta
	effPitch := float64(gyroPitch)

	dx := effYaw * g.cfg.SensYaw
	dy := effPitch * g.cfg.SensPitch

	if g.cfg.InvertPitch {
		dy = -dy
	}

	return float32(dx), float32(dy)
}
