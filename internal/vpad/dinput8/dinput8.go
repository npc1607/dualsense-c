package dinput8

import (
	"github.com/npc1607/dualsense-c/internal/vpad"
)

func init() {
	vpad.Register(vpad.ProtocolDInput8, Open)
}
