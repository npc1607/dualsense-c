// Package input parses DualSense input reports. This is what makes input-driven
// profiles (FPS, any game without telemetry) possible: the controller's own
// trigger travel and buttons are all the signal a rule needs.
package input

import "math"

// Report IDs. USB uses 0x01, Bluetooth 0x31; both carry the same payload with
// the Bluetooth report having one extra byte after the report ID.
const (
	usbReportID = 0x01
	btReportID  = 0x31
)

// State is one decoded controller frame. Analog triggers are 0..1, sticks are
// -1..1, buttons are 0/1.
type State struct {
	L2, R2      float32
	LX, LY      float32
	RX, RY      float32
	L1, R1      bool
	Cross       bool
	Circle      bool
	Square      bool
	Triangle    bool
	Up, Down    bool
	Left, Right bool
	PS          bool
	Touchpad    bool
	Mute        bool
	ReportID    byte
	Sequence    byte
}

// payloadOffset returns where the common payload starts for a report id.
func payloadOffset(id byte) (int, bool) {
	switch id {
	case usbReportID:
		return 1, true
	case btReportID:
		return 2, true
	}
	return 0, false
}

// Parse decodes a raw hidraw/HID input report. ok is false for reports that are
// not gamepad state (feature reports, short reads).
//
// Payload offsets (relative to payload start):
//
//	0..3  LX LY RX RY   (sticks, 0x80 = centre)
//	4     L2            (0..255 analog)
//	5     R2            (0..255 analog)
//	6     sequence
//	7..9  buttons0/1/2
//
// The analog trigger bytes are the load-bearing part of input-driven profiles;
// the button bit tables vary slightly between firmware revisions, so use
// `ds5ctl watch` to confirm them on your unit.
func Parse(b []byte) (State, bool) {
	var s State
	if len(b) < 12 {
		return s, false
	}
	s.ReportID = b[0]
	off, ok := payloadOffset(b[0])
	if !ok || len(b) < off+10 {
		return s, false
	}
	p := b[off:]
	if p[6] == 0 && p[0] == 0 && p[1] == 0 && p[4] == 0 && p[5] == 0 {
		// A zeroed payload is either a stale read or a non-gamepad report.
		return s, false
	}
	s.Sequence = p[6]

	s.LX = axis(p[0])
	s.LY = axis(p[1])
	s.RX = axis(p[2])
	s.RY = axis(p[3])
	s.L2 = float32(p[4]) / 255
	s.R2 = float32(p[5]) / 255

	b0, b1, b2 := p[7], p[8], p[9]
	s.Square = b0&0x01 != 0
	s.Cross = b0&0x02 != 0
	s.Circle = b0&0x04 != 0
	s.Triangle = b0&0x08 != 0
	s.L1 = b0&0x10 != 0
	s.R1 = b0&0x20 != 0

	s.PS = b1&0x10 != 0
	s.Touchpad = b1&0x20 != 0
	s.Mute = b1&0x40 != 0

	switch b2 & 0x0f {
	case 0:
		s.Up = true
	case 1:
		s.Up, s.Right = true, true
	case 2:
		s.Right = true
	case 3:
		s.Down, s.Right = true, true
	case 4:
		s.Down = true
	case 5:
		s.Down, s.Left = true, true
	case 6:
		s.Left = true
	case 7:
		s.Up, s.Left = true, true
	}
	return s, true
}

func axis(v byte) float32 {
	f := (float32(v) - 127.5) / 127.5
	if f > 1 {
		f = 1
	}
	if f < -1 {
		f = -1
	}
	return f
}

// Signals converts the state into the standard signal vocabulary.
//
// l2_full / r2_full are derived from analog travel rather than the digital
// button bits, which keeps them reliable across firmware revisions.
func (s State) Signals() map[string]float32 {
	b2f := func(b bool) float32 {
		if b {
			return 1
		}
		return 0
	}
	return map[string]float32{
		"l2":       s.L2,
		"r2":       s.R2,
		"l2_full":  boolToFloat(s.L2 >= 0.85),
		"r2_full":  boolToFloat(s.R2 >= 0.85),
		"l1":       b2f(s.L1),
		"r1":       b2f(s.R1),
		"cross":    b2f(s.Cross),
		"circle":   b2f(s.Circle),
		"square":   b2f(s.Square),
		"triangle": b2f(s.Triangle),
		"up":       b2f(s.Up),
		"down":     b2f(s.Down),
		"left":     b2f(s.Left),
		"right":    b2f(s.Right),
		"ps":       b2f(s.PS),
		"touchpad": b2f(s.Touchpad),
		"mute":     b2f(s.Mute),
		"lx":       s.LX,
		"ly":       s.LY,
		"rx":       s.RX,
		"ry":       s.RY,
		"stick_l":  float32(math.Hypot(float64(s.LX), float64(s.LY))),
		"stick_r":  float32(math.Hypot(float64(s.RX), float64(s.RY))),
	}
}

func boolToFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
