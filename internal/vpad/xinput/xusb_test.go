package xinput

import (
	"encoding/binary"
	"testing"

	"github.com/npc1607/dualsense-c/internal/input"
)

func TestEncodeXUSBReport(t *testing.T) {
	st := input.State{
		Cross:    true,
		Circle:   true,
		Square:   false,
		Triangle: true,
		L1:       true,
		R1:       false,
		L3:       true,
		R3:       false,
		Create:   true,
		Options:  true,
		PS:       true,
		Up:       true,
		Left:     true,
		L2:       1.0,
		R2:       0.5,
		LX:       0.01, // within deadzone 0.02 -> 0
		LY:       -1.0, // full up on DualSense (-1) -> +32767 on XInput
		RX:       1.0,  // full right -> +32767
		RY:       1.0,  // full down on DualSense (+1) -> -32768 on XInput
	}

	rep := encodeXUSBReport(st, 0.02)

	btns := binary.LittleEndian.Uint16(rep[0:2])
	wantBtns := uint16(xusbA | xusbB | xusbY | xusbLeftShoulder | xusbLeftThumb |
		xusbBack | xusbStart | xusbGuide | xusbDpadUp | xusbDpadLeft)
	if btns != wantBtns {
		t.Errorf("wButtons = %#04x, want %#04x", btns, wantBtns)
	}
	if rep[2] != 255 {
		t.Errorf("bLeftTrigger = %d, want 255", rep[2])
	}
	if rep[3] != 128 {
		t.Errorf("bRightTrigger = %d, want 128", rep[3])
	}
	lx := int16(binary.LittleEndian.Uint16(rep[4:6]))
	if lx != 0 {
		t.Errorf("sThumbLX = %d, want 0 (inside deadzone)", lx)
	}
	ly := int16(binary.LittleEndian.Uint16(rep[6:8]))
	if ly != 32767 {
		t.Errorf("sThumbLY = %d, want 32767 (inverted Y)", ly)
	}
	rx := int16(binary.LittleEndian.Uint16(rep[8:10]))
	if rx != 32767 {
		t.Errorf("sThumbRX = %d, want 32767", rx)
	}
	ry := int16(binary.LittleEndian.Uint16(rep[10:12]))
	if ry != -32768 {
		t.Errorf("sThumbRY = %d, want -32768 (inverted Y)", ry)
	}
}
