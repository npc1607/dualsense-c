package input

import (
	"encoding/binary"
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
	p := []byte{0x80, 0x80, 0x88, 0x77, 255, 128, 7, 0x22, 0x0f, 0x08}
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
	if !s.Create || !s.Options || !s.L3 || !s.R3 {
		t.Errorf("buttons1 = %#x: create=%v options=%v l3=%v r3=%v", p[8], s.Create, s.Options, s.L3, s.R3)
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

func TestParseSensors(t *testing.T) {
	b := make([]byte, 64)
	b[0] = 0x01
	p := b[1:]                                      // payload-relative offsets below
	p[0], p[1], p[2], p[3] = 0x80, 0x80, 0x80, 0x80 // sticks centred
	p[6] = 1                                        // sequence

	// gyro (payload 15) and accel (payload 21), int16 LE
	binary.LittleEndian.PutUint16(p[15:], uint16(1024))
	binary.LittleEndian.PutUint16(p[17:], uint16(0xF800)) // -2048
	binary.LittleEndian.PutUint16(p[23:], uint16(16384))

	// touch contact 1 at payload 32: active, id=3, x=960, y=540
	p[32] = 0x80 | 3
	p[33] = 960 & 0xff
	p[34] = byte((540&0x0f)<<4) | byte(960>>8)
	p[35] = byte(540 >> 4)

	// battery at payload 52: level 5, status nibble 1 (charging)
	p[52] = 0x15

	s, ok := Parse(b)
	if !ok {
		t.Fatal("Parse rejected the report")
	}
	if got := s.GyroX; got != 1024.0/32768 {
		t.Errorf("GyroX = %v", got)
	}
	if got := s.GyroY; got != -2048.0/32768 {
		t.Errorf("GyroY = %v", got)
	}
	if got := s.AccelY; got != 0.5 {
		t.Errorf("AccelY = %v, want 0.5", got)
	}
	if !s.TouchActive || s.TouchID != 3 {
		t.Errorf("touch active=%v id=%d", s.TouchActive, s.TouchID)
	}
	if math.Abs(float64(s.TouchX)-960.0/1919) > 1e-6 {
		t.Errorf("TouchX = %v", s.TouchX)
	}
	if math.Abs(float64(s.TouchY)-540.0/1079) > 1e-6 {
		t.Errorf("TouchY = %v", s.TouchY)
	}
	if s.Battery != 0.5 {
		t.Errorf("Battery = %v, want 0.5", s.Battery)
	}
	if !s.Charging {
		t.Error("Charging should be true for status nibble 1")
	}

	sig := s.Signals()
	for _, name := range []string{"gyro_x", "accel_y", "touch_x", "touch_y", "touch_active", "battery", "charging"} {
		if _, ok := sig[name]; !ok {
			t.Errorf("Signals() is missing %q", name)
		}
	}
}

func TestParseIgnoresInactiveTouch(t *testing.T) {
	b := make([]byte, 64)
	b[0] = 0x01
	p := b[1:]
	p[0], p[1], p[2], p[3] = 0x80, 0x80, 0x80, 0x80 // sticks centred
	p[6] = 1                                        // sequence
	p[32] = 0x03                                    // bit7 clear = no contact

	s, ok := Parse(b)
	if !ok {
		t.Fatal("Parse rejected the report")
	}
	if s.TouchActive {
		t.Error("touch should be inactive when bit7 is clear")
	}
	if s.TouchX != 0 || s.TouchY != 0 {
		t.Errorf("stale touch coords: %v,%v", s.TouchX, s.TouchY)
	}
}
