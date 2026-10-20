// Package all blank-imports all virtual gamepad protocol backends so they
// register themselves into internal/vpad.
package all

import (
	// Xbox 360 controller protocol (XInput standard)
	_ "github.com/npc1607/dualsense-c/internal/vpad/xinput"

	// Classic DirectInput 8 gamepad protocol (Logitech Dual Action 046d:c216)
	_ "github.com/npc1607/dualsense-c/internal/vpad/dinput8"
)
