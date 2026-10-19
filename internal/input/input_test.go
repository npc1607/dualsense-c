package input

import (
	"math"
	"testing"
)

// usbReport builds a 64-byte USB input report with the given payload.
func usbReport(p []byte) []byte {
	b := make([]byte, 64)
	b[0] = 0x01
	copy(b[1:], p)
	return b
}

func TestParseUSB(t *testing.T) {
	// payload: LX LY RX RY L2 R2 seq b0 b1 b2
	p := []byte{0x80, 0x80, 0x88, 0x77, 255, 128, 7, 0x22, 0x00, 0x08}
	s, ok := Parse(usbReport(p))
	if !ok {
		t.Fatal("Parse rejected a valid USB report")
	}
	if s.L2 != 1.0 {
		t.Errorf("L2 = %v, want 1", s.L2)
	}
	if got, want := s.R2, float32(128)/255; got != want {
		t.Errorf("R2 = %v, want %v", got, want)
	}
	if !s.Cross || !s.R1 {
		t.Errorf("buttons0 = %#x: cross=%v r1=%v", p[7], s.Cross, s.R1)
	}
	// 0x80 is the stick centre, so both axes must be ~0.
	if math.Abs(float64(s.LX)) > 0.01 || math.Abs(float64(s.LY)) > 0.01 {
		t.Errorf("centred sticks decoded as LX=%v LY=%v", s.LX, s.LY)
	}
	if s.RX <= 0 || s.RY >= 0 {
		t.Errorf("off-centre sticks decoded as RX=%v RY=%v", s.RX, s.RY)
	}
	if s.Sequence != 7 {
		t.Errorf("seq = %d", s.Sequence)
	}
}

func TestParseBTOffset(t *testing.T) {
	// BT: report id, one extra byte, then the same payload.
	b := make([]byte, 78)
	b[0], b[1] = 0x31, 0x01
	copy(b[2:], []byte{0x80, 0x80, 0x80, 0x80, 0, 200, 9, 0x00, 0x00, 0x02})
	s, ok := Parse(b)
	if !ok {
		t.Fatal("Parse rejected a valid BT report")
	}
	if got, want := s.R2, float32(200)/255; got != want {
		t.Errorf("R2 = %v, want %v", got, want)
	}
	if s.Sequence != 9 {
		t.Errorf("seq = %d", s.Sequence)
	}
	if !s.Right {
		t.Error("dpad right not detected")
	}
}

func TestParseRejectsJunk(t *testing.T) {
	if _, ok := Parse(make([]byte, 64)); ok {
		t.Error("all-zero report should be rejected")
	}
	if _, ok := Parse([]byte{0x05, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}); ok {
		t.Error("feature report id should be rejected")
	}
	if _, ok := Parse([]byte{0x01}); ok {
		t.Error("short report should be rejected")
	}
}

func TestSignalsExposeFullPress(t *testing.T) {
	p := []byte{0x80, 0x80, 0x80, 0x80, 250, 10, 1, 0, 0, 8}
	s, ok := Parse(usbReport(p))
	if !ok {
		t.Fatal("parse failed")
	}
	sig := s.Signals()
	if sig["l2_full"] != 1 || sig["r2_full"] != 0 {
		t.Errorf("full press flags = %v/%v", sig["l2_full"], sig["r2_full"])
	}
}
