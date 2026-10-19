// Package report picks the wire format for a device transport.
package report

import (
	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
)

// Build serialises a frame using the format the device expects.
func Build(info device.Info, f hid.Frame) []byte {
	if info.Transport == device.BT {
		return hid.BuildBT(f)
	}
	return hid.BuildUSB(f)
}
