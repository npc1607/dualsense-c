//go:build windows

package xinput

import (
	"fmt"

	"github.com/npc1607/dualsense-c/internal/vpad"
)

// Open is the Windows placeholder for the virtual Xbox 360 gamepad backend.
//
// Planned implementation: ViGEmBus client (ViGEmClient.TargetAdd Xbox360Controller).
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	return nil, fmt.Errorf("%w: Windows XInput backend is not implemented yet (planned: ViGEmBus)", vpad.ErrUnsupported)
}
