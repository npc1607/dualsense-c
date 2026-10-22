package xinput

import (
	"encoding/binary"
	"math"

	"github.com/npc1607/dualsense-c/internal/input"
)

// XUSB_BUTTON bitflags from ViGEm/Common.h (matches XInput).
const (
	xusbDpadUp        = 0x0001
	xusbDpadDown      = 0x0002
	xusbDpadLeft      = 0x0004
	xusbDpadRight     = 0x0008
	xusbStart         = 0x0010
	xusbBack          = 0x0020
	xusbLeftThumb     = 0x0040
	xusbRightThumb    = 0x0080
	xusbLeftShoulder  = 0x0100
	xusbRightShoulder = 0x0200
	xusbGuide         = 0x0400
	xusbA             = 0x1000
	xusbB             = 0x2000
	xusbX             = 0x4000
	xusbY             = 0x8000
)

// encodeXUSBReport packs a decoded DualSense input.State into the 12-byte
// XUSB_REPORT structure expected by ViGEmClient (and XInput).
func encodeXUSBReport(s input.State, deadzone float32) [12]byte {
	var btns uint16
	if s.Up {
		btns |= xusbDpadUp
	}
	if s.Down {
		btns |= xusbDpadDown
	}
	if s.Left {
		btns |= xusbDpadLeft
	}
	if s.Right {
		btns |= xusbDpadRight
	}
	if s.Options {
		btns |= xusbStart
	}
	if s.Create {
		btns |= xusbBack
	}
	if s.L3 {
		btns |= xusbLeftThumb
	}
	if s.R3 {
		btns |= xusbRightThumb
	}
	if s.L1 {
		btns |= xusbLeftShoulder
	}
	if s.R1 {
		btns |= xusbRightShoulder
	}
	if s.PS {
		btns |= xusbGuide
	}
	if s.Cross {
		btns |= xusbA
	}
	if s.Circle {
		btns |= xusbB
	}
	if s.Square {
		btns |= xusbX
	}
	if s.Triangle {
		btns |= xusbY
	}

	var rep [12]byte
	binary.LittleEndian.PutUint16(rep[0:], btns)
	rep[2] = scaleTrigger(s.L2)
	rep[3] = scaleTrigger(s.R2)
	// DualSense Y axes point down (+1 = down), while XInput thumbsticks point up
	// (+32767 = up), so LY and RY are inverted.
	binary.LittleEndian.PutUint16(rep[4:], uint16(scaleAxis(s.LX, deadzone)))
	binary.LittleEndian.PutUint16(rep[6:], uint16(scaleAxis(-s.LY, deadzone)))
	binary.LittleEndian.PutUint16(rep[8:], uint16(scaleAxis(s.RX, deadzone)))
	binary.LittleEndian.PutUint16(rep[10:], uint16(scaleAxis(-s.RY, deadzone)))
	return rep
}

func scaleTrigger(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(float64(v * 255)))
}

func scaleAxis(v, deadzone float32) int16 {
	if deadzone > 0 && float32(math.Abs(float64(v))) < deadzone {
		return 0
	}
	if v >= 1 {
		return 32767
	}
	if v <= -1 {
		return -32768
	}
	if v >= 0 {
		return int16(math.Round(float64(v * 32767)))
	}
	return int16(math.Round(float64(v * 32768)))
}
