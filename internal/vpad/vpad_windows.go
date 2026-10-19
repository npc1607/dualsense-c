//go:build windows

package vpad

import "fmt"

// Open is the Windows hook for the virtual pad.
//
// The plan for Windows is a ViGEm-style virtual bus driver (ViGEmBus, or
// Microsoft's own Xbox Gamepad virtual device API on newer builds):
//
//  1. create an Xbox 360 (or Xbox One) virtual pad through the bus driver;
//  2. read the physical DualSense through an exclusive HID handle and forward
//     input to the virtual pad;
//  3. implement the bus driver's rumble callback. Unlike Linux's uinput, ViGEm
//     reports playback directly, so `source/ffb` gets real rumble intensity
//     with no heuristics;
//  4. keep publishing through the same RumbleSink type, so nothing above this
//     file changes.
//
// Until that exists, ds5vpad refuses to start and `source/ffb` simply receives
// nothing on this platform.
func Open(cfg Config, sink RumbleSink) (Pad, error) {
	return nil, fmt.Errorf("%w: Windows backend is not implemented yet (planned: ViGEmBus)", ErrUnsupported)
}

// DeviceExists is only meaningful on Linux today.
func DeviceExists(name string) bool { return false }
