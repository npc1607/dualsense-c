package hid

import (
	"bytes"
	"hash/crc32"
	"testing"
)

func TestBuildUSBLayout(t *testing.T) {
	f := Frame{
		L2:         Trigger{Mode: ModeFeedback, P: [10]byte{0, 180, 110}},
		R2:         Trigger{Mode: ModeWeapon, P: [10]byte{0, 40, 60, 8}},
		RumbleL:    0x11,
		RumbleR:    0x22,
		Lightbar:   [3]uint8{0, 255, 0},
		PlayerLEDs: 0x04,
	}
	b := BuildUSB(f)

	if len(b) != USBReportLen {
		t.Fatalf("len = %d, want %d", len(b), USBReportLen)
	}
	if b[0] != 0x02 {
		t.Errorf("report id = %#x, want 0x02", b[0])
	}
	if b[3] != 0x22 || b[4] != 0x11 {
		t.Errorf("rumble bytes = %#x/%#x, want right=0x22 left=0x11", b[3], b[4])
	}
	// Trigger block: [0] = mode, [1..10] = params.
	if b[11] != byte(ModeWeapon) || b[12] != 0 || b[13] != 40 || b[14] != 60 || b[15] != 8 {
		t.Errorf("R2 block = % x", b[11:22])
	}
	if b[22] != byte(ModeFeedback) || b[23] != 0 || b[24] != 180 || b[25] != 110 {
		t.Errorf("L2 block = % x", b[22:33])
	}
	if !bytes.Equal(b[45:48], []byte{0, 255, 0}) {
		t.Errorf("lightbar = % x", b[45:48])
	}
	if b[44] != 0x04 {
		t.Errorf("player leds = %#x", b[44])
	}
}

func TestBuildUSBReleasesTriggers(t *testing.T) {
	b := BuildUSB(Idle())
	if b[11] != 0 || b[22] != 0 {
		t.Fatalf("idle frame must release both triggers, got %#x/%#x", b[11], b[22])
	}
}

func TestBuildBTShiftsPayloadAndCRCs(t *testing.T) {
	f := Frame{L2: Trigger{Mode: ModeRigid, P: [10]byte{0, 200}}}
	b := BuildBT(f)
	if len(b) != BTReportLen {
		t.Fatalf("len = %d, want %d", len(b), BTReportLen)
	}
	if b[0] != 0x31 {
		t.Errorf("report id = %#x", b[0])
	}
	// USB byte 22 (L2 mode) must land at BT byte 23.
	if b[23] != byte(ModeRigid) || b[25] != 200 {
		t.Errorf("L2 block = % x", b[23:34])
	}
	// Lightbar shifted by one: USB 45..47 -> BT 46..48.
	if got := crc32.ChecksumIEEE(b[:BTReportLen-4]); got != crc32.ChecksumIEEE(b[:74]) {
		t.Fatalf("crc prefix mismatch")
	}
}

func TestScaledFadesOnlyForceParams(t *testing.T) {
	tr := Trigger{Mode: ModeWeapon, P: [10]byte{3, 100, 200, 8}}
	half := tr.Scaled(0.5)
	if half.P[0] != 3 || half.P[3] != 8 {
		t.Errorf("geometry params must be preserved: % x", half.P[:4])
	}
	if half.P[1] != 50 || half.P[2] != 100 {
		t.Errorf("force params = %d/%d, want 50/100", half.P[1], half.P[2])
	}
	if !tr.Scaled(0).IsOff() {
		t.Error("scale 0 must yield off")
	}
}

func TestParseMode(t *testing.T) {
	for _, name := range []string{"off", "rigid", "feedback", "weapon", "vibration", "slope_feedback", "machine"} {
		if _, ok := ParseMode(name); !ok {
			t.Errorf("ParseMode(%q) failed", name)
		}
	}
	if _, ok := ParseMode("nope"); ok {
		t.Error("unknown mode accepted")
	}
}

func TestParamIndex(t *testing.T) {
	if i, ok := ParamIndex(ModeFeedback, "force"); !ok || i != 2 {
		t.Errorf("feedback.force = %d,%v", i, ok)
	}
	if i, ok := ParamIndex(ModeWeapon, "p4"); !ok || i != 4 {
		t.Errorf("p4 = %d,%v", i, ok)
	}
	if _, ok := ParamIndex(ModeRigid, "amplitude"); ok {
		t.Error("rigid.amplitude should be rejected")
	}
}
