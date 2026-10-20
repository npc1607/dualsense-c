//go:build windows

package dinput8

import (
	"fmt"

	"github.com/npc1607/dualsense-c/internal/vpad"
)

// Open is the Windows placeholder for the DirectInput 8 virtual pad.
//
// Planned implementation:
//  1. vJoy feeder API (vJoyInterface.dll): feeds an 8-axis 32-button generic DirectInput 8 device.
//  2. ViGEmBus DualShock 4 target: Windows natively enumerates it as a DirectInput 8 controller
//     without trigger merging or XInput translation layers.
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	return nil, fmt.Errorf("%w: Windows DirectInput8 backend is not implemented yet (planned: vJoy / ViGEm DS4)", vpad.ErrUnsupported)
}
