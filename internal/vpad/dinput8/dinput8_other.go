//go:build !linux && !windows

package dinput8

import (
	"fmt"

	"github.com/npc1607/dualsense-c/internal/vpad"
)

func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	return nil, fmt.Errorf("%w: no DirectInput8 backend for this OS", vpad.ErrUnsupported)
}
