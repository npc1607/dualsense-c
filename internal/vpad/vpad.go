// Package vpad presents a virtual gamepad to the game so that the project can
// own the physical controller's output path.
//
// Why this exists: when Steam Input is enabled, Steam creates its own virtual
// Xbox pad and also writes the physical DualSense's HID output. A tool that
// wants to drive the triggers and voice coils then fights Steam for the device.
// ds5vpad inverts that: it builds the virtual pad itself, grabs the physical
// controller exclusively, and forwards input. Steam Input can stay off, the game
// still sees a normal Xbox pad, and nothing else writes to the controller.
//
// It is also the only place a game's force-feedback requests can be observed:
// the game (or Proton's XInput translation) sends them to the virtual device.
//
// Platform support:
//   - Linux: implemented with /dev/uinput (see vpad_linux.go).
//   - Windows: not implemented yet; the intended path is a ViGEm-style virtual
//     bus driver, which reports rumble callbacks natively. See vpad_windows.go.
package vpad

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned on platforms without a virtual-pad backend yet.
var ErrUnsupported = errors.New("vpad: virtual gamepad backend not available on this platform")

// Rumble is a force-feedback state reported by the game, 0..255 per motor.
type Rumble struct {
	Left, Right uint8
	// At is when the state was last refreshed; the FF source uses it to expire
	// a rumble that stopped being played.
	At time.Time
}

// RumbleSource is where captured force-feedback is published. Every backend
// calls it whenever the game changes the rumble state, so a future Windows
// backend only has to feed the same channel.
type RumbleSink func(Rumble)

// Config describes the virtual pad.
type Config struct {
	// Name is the device name the game sees.
	Name string
	// Vendor/Product identify the pad. 045e:028e is the wired Xbox 360 pad,
	// which every SDL/Wine build already has a mapping for.
	Vendor, Product uint16
	// GrabPath forces a specific physical evdev node to forward. Empty means
	// auto-detect the first DualSense gamepad.
	GrabPath string
	// PlayerLEDs, Deadzone are optional tuning knobs for the input mapping.
	Deadzone float32
	// Logger receives diagnostics.
	Logger interface {
		Printf(format string, args ...any)
	}
}

// DefaultConfig returns the Xbox 360 identity plus auto-detection.
func DefaultConfig() Config {
	return Config{
		Name:     "ds5vpad virtual pad",
		Vendor:   0x045e,
		Product:  0x028e,
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
