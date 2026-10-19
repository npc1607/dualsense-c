// Package input parses DualSense input reports. This is what makes input-driven
// profiles (FPS, any game without telemetry) possible: the controller's own
// trigger travel and buttons are all the signal a rule needs.
package input

import (
	"encoding/binary"
	"math"
)

// Report IDs. USB uses 0x01, Bluetooth 0x31; both carry the same payload with
// the Bluetooth report having one extra byte after the report ID.
const (
	usbReportID = 0x01
	btReportID  = 0x31
)

// State is one decoded controller frame. Analog triggers are 0..1, sticks are
// -1..1, buttons are 0/1.
//
// The sensor fields (gyro/accel/touch/battery) follow the community
// documentation and the kernel's dualsense_input_report struct. They have not
// been verified against every firmware revision -- run `ds5ctl watch` and check
// that the values react as expected before relying on them in a profile.
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

	// Motion sensors, normalised to full scale (-1..1). The report carries raw
	// int16 values; scale them in a profile if you need physical units.
	GyroX, GyroY, GyroZ    float32
	AccelX, AccelY, AccelZ float32

	// Touchpad contact (first active finger), 0..1 across the 1920x1080 pad.
	TouchActive bool
	TouchID     byte
	TouchX      float32
	TouchY      float32

	// Battery level 0..1 in 10% steps, plus a charging flag. BatteryRaw keeps
	// the status byte so `ds5ctl watch` can be used to calibrate the nibbles.
	Battery    float32
	Charging   bool
	BatteryRaw byte
}

// Payload-relative offsets of the sensor block. The USB report starts the
// payload at byte 1 and Bluetooth at byte 2, so the same indices work for both.
const (
	offGyro        = 15 // 3 x int16 LE: pitch, yaw, roll
	offAccel       = 21 // 3 x int16 LE
	offTouch1      = 32 // 4 bytes: id/active, xlo, ylo|xhi, yhi
	offTouch2      = 36
	offBattery     = 52 // low nibble = level (0..10), high nibble = status
	touchpadWidth  = 1920
	touchpadHeight = 1080
)

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

	// Sensor block: only present in full-length reports, and its offsets are
	// unverified on some firmware, so failures here must not reject the frame.
	if len(p) >= offAccel+6 {
		s.GyroX = s16(p, offGyro)
		s.GyroY = s16(p, offGyro+2)
		s.GyroZ = s16(p, offGyro+4)
		s.AccelX = s16(p, offAccel)
		s.AccelY = s16(p, offAccel+2)
		s.AccelZ = s16(p, offAccel+4)
	}
	if len(p) >= offTouch2+4 {
		if id, x, y, ok := parseTouch(p[offTouch1 : offTouch1+4]); ok {
			s.TouchActive = true
			s.TouchID = id
			s.TouchX = x
			s.TouchY = y
		} else if id, x, y, ok := parseTouch(p[offTouch2 : offTouch2+4]); ok {
			s.TouchActive = true
			s.TouchID = id
			s.TouchX = x
			s.TouchY = y
		}
	}
	if len(p) > offBattery {
		s.BatteryRaw = p[offBattery]
		level := p[offBattery] & 0x0f
		if level <= 10 {
			s.Battery = float32(level) / 10
		}
		switch (p[offBattery] >> 4) & 0x0f {
		case 1, 2: // charging, full
			s.Charging = true
		}
	}

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

// s16 reads a little-endian int16 and normalises it to full scale.
func s16(b []byte, off int) float32 {
	if off+2 > len(b) {
		return 0
	}
	return float32(int16(binary.LittleEndian.Uint16(b[off:]))) / 32768
}

// parseTouch decodes one 4-byte touch contact.
//
//	byte 0: bit7 = active, bits0-6 = touch id
//	byte 1: x low byte
//	byte 2: bits0-3 = x high nibble, bits4-7 = y low nibble
//	byte 3: y high byte
func parseTouch(t []byte) (id byte, x, y float32, ok bool) {
	if t[0]&0x80 == 0 {
		return 0, 0, 0, false
	}
	id = t[0] & 0x7f
	rawX := int(t[1]) | int(t[2]&0x0f)<<8
	rawY := int(t[2]>>4) | int(t[3])<<4
	x = clamp01f(float32(rawX) / float32(touchpadWidth-1))
	y = clamp01f(float32(rawY) / float32(touchpadHeight-1))
	return id, x, y, true
}

func clamp01f(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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

		"gyro_x":  s.GyroX,
		"gyro_y":  s.GyroY,
		"gyro_z":  s.GyroZ,
		"accel_x": s.AccelX,
		"accel_y": s.AccelY,
		"accel_z": s.AccelZ,

		"touch_x":      s.TouchX,
		"touch_y":      s.TouchY,
		"touch_active": boolToFloat(s.TouchActive),

		"battery":  s.Battery,
		"charging": boolToFloat(s.Charging),
	}
}

func boolToFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
