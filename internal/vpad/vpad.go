// Package vpad presents a virtual gamepad to the game so that the project can
// own the physical controller's output path.
//
// Why this exists: when Steam Input is enabled, Steam creates its own virtual
// Xbox pad and also writes the physical DualSense's HID output. A tool that
// wants to drive the triggers and voice coils then fights Steam for the device.
// ds5vpad inverts that: it builds the virtual pad itself, grabs the physical
// controller exclusively, and forwards input. Steam Input can stay off, the game
// still sees a normal pad, and nothing else writes to the controller.
//
// Supported protocol families:
//   - XInput (Xbox 360, 045e:028e): modern PC games, standard Steam/Proton layout.
//   - DirectInput 8 (Logitech Dual Action, 046d:c216): classic pre-XInput (2000s era)
//     racing and action games like Richard Burns Rally, with independent analog
//     pedal axes and discrete DirectInput buttons.
//
// Platform support:
//   - Linux: implemented via /dev/uinput.
//   - Windows: implemented with ViGEmBus / ViGEmClient.dll (XInput Xbox 360 & DirectInput 8 DualShock 4).
package vpad

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/npc1607/dualsense-c/internal/aim"
	"github.com/npc1607/dualsense-c/internal/profile"
)

// ErrUnsupported is returned on platforms without a virtual-pad backend yet.
var ErrUnsupported = errors.New("vpad: virtual gamepad backend not available on this platform")

// Protocol selects the hardware protocol simulated by the virtual pad.
type Protocol string

const (
	// ProtocolXInput simulates a wired Xbox 360 pad (045e:028e), standard for modern games.
	ProtocolXInput Protocol = "xinput"

	// ProtocolDInput8 simulates a classic DirectInput 8 gamepad (Logitech Dual Action 046d:c216),
	// featuring independent unipolar pedal axes (L2 brake, R2 throttle) and discrete DInput buttons.
	ProtocolDInput8 Protocol = "dinput8"
)

// Rumble is a force-feedback state reported by the game, 0..255 per motor.
type Rumble struct {
	Left, Right uint8
	// At is when the state was last refreshed; the FF source uses it to expire
	// a rumble that stopped being played.
	At time.Time
}

// RumbleSink is where captured force-feedback is published. Every backend
// calls it whenever the game changes the rumble state, so a future Windows
// backend only has to feed the same channel.
type RumbleSink func(Rumble)

// Config describes the virtual pad.
type Config struct {
	// Protocol selects the virtual gamepad protocol (xinput or dinput8).
	Protocol Protocol

	// Name is the device name the game sees. If empty, the protocol's default is used.
	Name string

	// Vendor/Product identify the pad. If zero, the protocol's default is used.
	Vendor, Product uint16

	// GrabPath forces a specific physical evdev node to forward. Empty means
	// auto-detect the first DualSense gamepad.
	GrabPath string

	// Deadzone is an optional tuning knob for the input mapping.
	Deadzone float32

	// Aim holds optional stick curves, gyro blending, and anti-recoil configuration.
	Aim *profile.AimConfig

	// Logger receives diagnostics.
	Logger interface {
		Printf(format string, args ...any)
	}
}

// DefaultConfig returns the default XInput (Xbox 360) identity.
func DefaultConfig() Config {
	return Config{
		Protocol: ProtocolXInput,
		Name:     "ds5vpad virtual pad",
		Vendor:   0x045e,
		Product:  0x028e,
		Deadzone: 0.02,
	}
}

// DInput8Config returns a classic DirectInput 8 identity (Logitech Dual Action).
// Note: On Windows, the ViGEmBus backend emulates a DualShock 4 target and
// automatically retains Sony's native VID/PID (0x054c:0x05c4) so Windows loads
// hidclass.sys/hidusb.sys correctly.
func DInput8Config() Config {
	return Config{
		Protocol: ProtocolDInput8,
		Name:     "Logitech Dual Action (DInput8)",
		Vendor:   0x046d,
		Product:  0xc216,
		Deadzone: 0.02,
	}
}

// Pad is a running virtual gamepad.
type Pad interface {
	// Name is the OS device the game will see.
	Name() string
	// Run forwards input until ctx is cancelled.
	Run(ctx context.Context) error
	// Close destroys the virtual device.
	Close() error
}

// Backend creates a Pad instance for a specific protocol.
type Backend func(cfg Config, sink RumbleSink) (Pad, error)

var (
	registryMu sync.RWMutex
	registry   = map[Protocol]Backend{}
)

// Register makes a virtual pad protocol backend available.
func Register(proto Protocol, b Backend) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[proto]; dup {
		panic("vpad: duplicate registration for protocol " + string(proto))
	}
	registry[proto] = b
}

// Open builds the virtual pad using the backend registered for cfg.Protocol.
func Open(cfg Config, sink RumbleSink) (Pad, error) {
	if cfg.Protocol == "" {
		cfg.Protocol = ProtocolXInput
	}
	registryMu.RLock()
	b, ok := registry[cfg.Protocol]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("vpad: unsupported protocol %q (did you blank-import vpad/all?)", cfg.Protocol)
	}
	return b(cfg, sink)
}

// NewAimProcessor creates an aim.Processor from a profile.AimConfig, or nil if cfg is nil.
func NewAimProcessor(cfg *profile.AimConfig) *aim.Processor {
	if cfg == nil {
		return nil
	}
	return aim.NewProcessor(
		aim.CurveConfig{
			Type:          aim.CurveType(cfg.Curve.Type),
			Exponent:      cfg.Curve.Exponent,
			MidPoint:      cfg.Curve.MidPoint,
			InnerDeadzone: cfg.Curve.InnerDeadzone,
			OuterBoost:    cfg.Curve.OuterBoost,
		},
		aim.FilterConfig{
			Type:      aim.FilterType(cfg.Filter.Type),
			MinCutoff: cfg.Filter.MinCutoff,
			Beta:      cfg.Filter.Beta,
			DCutoff:   cfg.Filter.DCutoff,
		},
		aim.GyroConfig{
			Enabled:          cfg.Gyro.Enabled,
			Trigger:          aim.GyroTriggerType(cfg.Gyro.Trigger),
			TriggerThreshold: cfg.Gyro.TriggerThreshold,
			NaturalAngleDeg:  cfg.Gyro.NaturalAngleDeg,
			DeadzoneDPS:      cfg.Gyro.DeadzoneDPS,
			SensYaw:          cfg.Gyro.SensYaw,
			SensPitch:        cfg.Gyro.SensPitch,
			InvertPitch:      cfg.Gyro.InvertPitch,
		},
		aim.AntiRecoilConfig{
			Enabled:      cfg.AntiRecoil.Enabled,
			Trigger:      cfg.AntiRecoil.Trigger,
			StartDelay:   cfg.AntiRecoil.StartDelay.Duration(),
			BurstForce:   cfg.AntiRecoil.BurstForce,
			BurstDecay:   cfg.AntiRecoil.BurstDecay,
			SustainForce: cfg.AntiRecoil.SustainForce,
			JitterNoise:  cfg.AntiRecoil.JitterNoise,
		},
	)
}
