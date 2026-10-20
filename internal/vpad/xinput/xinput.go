package xinput

import (
	"github.com/npc1607/dualsense-c/internal/vpad"
)

func init() {
	vpad.Register(vpad.ProtocolXInput, Open)
}
