package dinput8

import (
	"encoding/binary"
	"testing"

	"github.com/npc1607/dualsense-c/internal/input"
)

func TestEncodeDS4Report_Center(t *testing.T) {
	st := input.State{}
	rep := encodeDS4Report(st, 0.02)

	// Neutral sticks -> 128
	if rep[0] != 128 {
		t.Errorf("rep[0] (LX) = %d, want 128", rep[0])
	}
	if rep[1] != 128 {
		t.Errorf("rep[1] (LY) = %d, want 128", rep[1])
	}
	if rep[2] != 128 {
		t.Errorf("rep[2] (RX) = %d, want 128", rep[2])
	}
	if rep[3] != 128 {
		t.Errorf("rep[3] (RY) = %d, want 128", rep[3])
	}

	// Neutral D-pad -> ds4DpadNone (0x8 in bits 0..3), buttons 0
	btns := binary.LittleEndian.Uint16(rep[4:6])
	if btns != ds4DpadNone {
		t.Errorf("wButtons = %#04x, want %#04x (ds4DpadNone)", btns, ds4DpadNone)
	}

	// bSpecial -> 0
	if rep[6] != 0 {
		t.Errorf("bSpecial = %d, want 0", rep[6])
	}

	// Triggers -> 0
	if rep[7] != 0 {
		t.Errorf("bTriggerL = %d, want 0", rep[7])
	}
	if rep[8] != 0 {
		t.Errorf("bTriggerR = %d, want 0", rep[8])
	}

	// Padding byte -> 0
	if rep[9] != 0 {
		t.Errorf("rep[9] (padding) = %d, want 0", rep[9])
	}
}

func TestEncodeDS4Report_Deadzone(t *testing.T) {
	st := input.State{
		LX: 0.03,
		LY: -0.04,
		RX: -0.02,
		RY: 0.049,
	}
	rep := encodeDS4Report(st, 0.05)

	if rep[0] != 128 {
		t.Errorf("LX within deadzone: rep[0] = %d, want 128", rep[0])
	}
	if rep[1] != 128 {
		t.Errorf("LY within deadzone: rep[1] = %d, want 128", rep[1])
	}
	if rep[2] != 128 {
		t.Errorf("RX within deadzone: rep[2] = %d, want 128", rep[2])
	}
	if rep[3] != 128 {
		t.Errorf("RY within deadzone: rep[3] = %d, want 128", rep[3])
	}
}

func TestEncodeDS4Report_Extremes(t *testing.T) {
	stMin := input.State{
		LX: -1.0,
		LY: -1.0,
		RX: -1.0,
		RY: -1.0,
	}
	repMin := encodeDS4Report(stMin, 0.02)
	for i := 0; i < 4; i++ {
		if repMin[i] != 0 {
			t.Errorf("repMin[%d] = %d, want 0", i, repMin[i])
		}
	}

	stMax := input.State{
		LX: 1.0,
		LY: 1.0,
		RX: 1.0,
		RY: 1.0,
	}
	repMax := encodeDS4Report(stMax, 0.02)
	for i := 0; i < 4; i++ {
		if repMax[i] != 255 {
			t.Errorf("repMax[%d] = %d, want 255", i, repMax[i])
		}
	}
}

func TestEncodeDS4Report_Dpad(t *testing.T) {
	tests := []struct {
		name                  string
		up, down, left, right bool
		wantHat               uint16
	}{
		{"Neutral", false, false, false, false, ds4DpadNone},
		{"North", true, false, false, false, ds4DpadNorth},
		{"NorthEast", true, false, false, true, ds4DpadNorthEast},
		{"East", false, false, false, true, ds4DpadEast},
		{"SouthEast", false, true, false, true, ds4DpadSouthEast},
		{"South", false, true, false, false, ds4DpadSouth},
		{"SouthWest", false, true, true, false, ds4DpadSouthWest},
		{"West", false, false, true, false, ds4DpadWest},
		{"NorthWest", true, false, true, false, ds4DpadNorthWest},
		{"Opposing Up/Down", true, true, false, false, ds4DpadNone},
		{"Opposing Left/Right", false, false, true, true, ds4DpadNone},
		{"All Four", true, true, true, true, ds4DpadNone},
		{"Up+Down with Right", true, true, false, true, ds4DpadEast},
		{"Left+Right with Up", true, false, true, true, ds4DpadNorth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := input.State{
				Up:    tt.up,
				Down:  tt.down,
				Left:  tt.left,
				Right: tt.right,
			}
			rep := encodeDS4Report(st, 0)
			btns := binary.LittleEndian.Uint16(rep[4:6])
			hat := btns & 0x0F
			if hat != tt.wantHat {
				t.Errorf("D-pad hat = %#x, want %#x", hat, tt.wantHat)
			}
		})
	}
}

func TestEncodeDS4Report_Buttons(t *testing.T) {
	buttonTests := []struct {
		name    string
		set     func(*input.State)
		wantBtn uint16
		wantSpc uint8
	}{
		{"Square", func(s *input.State) { s.Square = true }, ds4ButtonSquare, 0},
		{"Cross", func(s *input.State) { s.Cross = true }, ds4ButtonCross, 0},
		{"Circle", func(s *input.State) { s.Circle = true }, ds4ButtonCircle, 0},
		{"Triangle", func(s *input.State) { s.Triangle = true }, ds4ButtonTriangle, 0},
		{"L1", func(s *input.State) { s.L1 = true }, ds4ButtonL1, 0},
		{"R1", func(s *input.State) { s.R1 = true }, ds4ButtonR1, 0},
		{"Create/Share", func(s *input.State) { s.Create = true }, ds4ButtonShare, 0},
		{"Options", func(s *input.State) { s.Options = true }, ds4ButtonOptions, 0},
		{"L3", func(s *input.State) { s.L3 = true }, ds4ButtonL3, 0},
		{"R3", func(s *input.State) { s.R3 = true }, ds4ButtonR3, 0},
		{"PS", func(s *input.State) { s.PS = true }, 0, ds4SpecialPS},
		{"Touchpad", func(s *input.State) { s.Touchpad = true }, 0, ds4SpecialTouchpad},
	}

	for _, tt := range buttonTests {
		t.Run(tt.name, func(t *testing.T) {
			var st input.State
			tt.set(&st)
			rep := encodeDS4Report(st, 0)
			btns := binary.LittleEndian.Uint16(rep[4:6])
			actionBtns := btns & 0xFFF0
			if actionBtns != tt.wantBtn {
				t.Errorf("action buttons = %#04x, want %#04x", actionBtns, tt.wantBtn)
			}
			if rep[6] != tt.wantSpc {
				t.Errorf("bSpecial = %#02x, want %#02x", rep[6], tt.wantSpc)
			}
		})
	}

	// Digital L2/R2 bit isolation test:
	// Verify that depressing analog triggers does NOT set ds4ButtonL2 (1<<10) or ds4ButtonR2 (1<<11).
	t.Run("DigitalTriggerBitIsolation", func(t *testing.T) {
		st := input.State{
			L2: 1.0,
			R2: 1.0,
		}
		rep := encodeDS4Report(st, 0)
		btns := binary.LittleEndian.Uint16(rep[4:6])
		if (btns & ds4ButtonL2) != 0 {
			t.Errorf("ds4ButtonL2 (bit 10) was set, should remain 0 for analog pedal purity")
		}
		if (btns & ds4ButtonR2) != 0 {
			t.Errorf("ds4ButtonR2 (bit 11) was set, should remain 0 for analog pedal purity")
		}
	})
}

func TestEncodeDS4Report_IndependentPedals(t *testing.T) {
	st := input.State{
		L2: 1.0,
		R2: 0.5,
	}
	rep := encodeDS4Report(st, 0)

	if rep[7] != 255 {
		t.Errorf("bTriggerL = %d, want 255", rep[7])
	}
	if rep[8] != 128 {
		t.Errorf("bTriggerR = %d, want 128", rep[8])
	}

	// Cross-talk test: zero L2, full R2
	st2 := input.State{
		L2: 0.0,
		R2: 1.0,
	}
	rep2 := encodeDS4Report(st2, 0)
	if rep2[7] != 0 {
		t.Errorf("bTriggerL = %d, want 0 when L2=0", rep2[7])
	}
	if rep2[8] != 255 {
		t.Errorf("bTriggerR = %d, want 255 when R2=1.0", rep2[8])
	}
}
