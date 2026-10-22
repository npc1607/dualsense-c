package dinput8

import (
	"encoding/binary"
	"math"

	"github.com/npc1607/dualsense-c/internal/input"
)

// D-Pad Hat Switch (wButtons bits 0..3):
const (
	ds4DpadNorth     = 0x0
	ds4DpadNorthEast = 0x1
	ds4DpadEast      = 0x2
	ds4DpadSouthEast = 0x3
	ds4DpadSouth     = 0x4
	ds4DpadSouthWest = 0x5
	ds4DpadWest      = 0x6
	ds4DpadNorthWest = 0x7
	ds4DpadNone      = 0x8 // Neutral / released
)

// wButtons bits 4..15 (Action buttons):
const (
	ds4ButtonSquare   = 1 << 4  // DirectInput Button 1
	ds4ButtonCross    = 1 << 5  // DirectInput Button 2
	ds4ButtonCircle   = 1 << 6  // DirectInput Button 3
	ds4ButtonTriangle = 1 << 7  // DirectInput Button 4
	ds4ButtonL1       = 1 << 8  // DirectInput Button 5
	ds4ButtonR1       = 1 << 9  // DirectInput Button 6
	ds4ButtonL2       = 1 << 10 // Omitted in encoder to preserve analog pedal purity
	ds4ButtonR2       = 1 << 11 // Omitted in encoder to preserve analog pedal purity
	ds4ButtonShare    = 1 << 12 // DirectInput Button 9 (Create)
	ds4ButtonOptions  = 1 << 13 // DirectInput Button 10
	ds4ButtonL3       = 1 << 14 // DirectInput Button 11
	ds4ButtonR3       = 1 << 15 // DirectInput Button 12
)

// bSpecial bits 0..1:
const (
	ds4SpecialPS       = 1 << 0 // DirectInput Button 13
	ds4SpecialTouchpad = 1 << 1 // DirectInput Button 14
)

// scaleDS4Axis scales DualSense thumbstick -1..1 into DS4 0..255 (128 centered).
func scaleDS4Axis(v, deadzone float32) uint8 {
	if deadzone > 0 && float32(math.Abs(float64(v))) < deadzone {
		return 128
	}
	if v <= -1 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(127.5 + float64(v)*127.5))
}

// scaleDS4Trigger scales DualSense analog trigger 0..1 into unipolar pedal 0..255.
func scaleDS4Trigger(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(float64(v * 255)))
}

// ds4Dpad resolves DualSense Up/Down/Left/Right into the DS4 4-bit POV hat nibble.
func ds4Dpad(up, down, left, right bool) uint16 {
	u := up && !down
	d := down && !up
	l := left && !right
	r := right && !left
	switch {
	case u && r:
		return ds4DpadNorthEast
	case d && r:
		return ds4DpadSouthEast
	case d && l:
		return ds4DpadSouthWest
	case u && l:
		return ds4DpadNorthWest
	case u:
		return ds4DpadNorth
	case r:
		return ds4DpadEast
	case d:
		return ds4DpadSouth
	case l:
		return ds4DpadWest
	default:
		return ds4DpadNone
	}
}

// encodeDS4Report serializes input.State into the 10-byte DS4_REPORT layout.
//
// In ViGEmClient, DS4_REPORT is defined as:
//
//	BYTE bThumbLX;
//	BYTE bThumbLY;
//	BYTE bThumbRX;
//	BYTE bThumbRY;
//	WORD wButtons;
//	BYTE bSpecial;
//	BYTE bTriggerL;
//	BYTE bTriggerR;
//
// Which packs into 10 bytes with 2-byte alignment padding at the end.
func encodeDS4Report(s input.State, deadzone float32) [10]byte {
	btns := ds4Dpad(s.Up, s.Down, s.Left, s.Right)
	if s.Square {
		btns |= ds4ButtonSquare
	}
	if s.Cross {
		btns |= ds4ButtonCross
	}
	if s.Circle {
		btns |= ds4ButtonCircle
	}
	if s.Triangle {
		btns |= ds4ButtonTriangle
	}
	if s.L1 {
		btns |= ds4ButtonL1
	}
	if s.R1 {
		btns |= ds4ButtonR1
	}
	// Note: ds4ButtonL2 and ds4ButtonR2 are deliberately omitted to preserve
	// analog pedal purity in classic DirectInput 8 games (e.g. Richard Burns Rally).
	if s.Create {
		btns |= ds4ButtonShare
	}
	if s.Options {
		btns |= ds4ButtonOptions
	}
	if s.L3 {
		btns |= ds4ButtonL3
	}
	if s.R3 {
		btns |= ds4ButtonR3
	}

	var special uint8
	if s.PS {
		special |= ds4SpecialPS
	}
	if s.Touchpad {
		special |= ds4SpecialTouchpad
	}

	var rep [10]byte
	rep[0] = scaleDS4Axis(s.LX, deadzone)
	rep[1] = scaleDS4Axis(s.LY, deadzone)
	rep[2] = scaleDS4Axis(s.RX, deadzone)
	rep[3] = scaleDS4Axis(s.RY, deadzone)
	binary.LittleEndian.PutUint16(rep[4:6], btns)
	rep[6] = special
	rep[7] = scaleDS4Trigger(s.L2)
	rep[8] = scaleDS4Trigger(s.R2)
	rep[9] = 0 // Tail padding for struct 2-byte alignment
	return rep
}
