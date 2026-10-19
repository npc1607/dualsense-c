package hid

import (
	"encoding/binary"
	"hash/crc32"
)

// Report sizes. USB output reports are 48 bytes, Bluetooth reports are a
// 0x31 report of 78 bytes with a trailing CRC32.
const (
	USBReportLen = 48
	BTReportLen  = 78

	usbReportID = 0x02
	btReportID  = 0x31

	// Offsets inside the USB report (matching the kernel's
	// dualsense_output_report_common).
	offFlags0    = 1
	offFlags1    = 2
	offRumbleR   = 3
	offRumbleL   = 4
	offMuteLED   = 9
	offR2Block   = 11
	offL2Block   = 22
	offValid2    = 39
	offBright    = 43
	offPlayerLED = 44
	offLightbar  = 45
)

// Frame is the complete controller state for one output tick. It is the only
// thing the sink needs: no game knowledge, no telemetry types.
type Frame struct {
	L2, R2           Trigger
	RumbleL, RumbleR uint8
	// HapticL/HapticR are amplitudes for the voice-coil actuators, driven over
	// the controller's USB audio channels 3/4 instead of the HID rumble bytes.
	// The HID path only has "amplitude + one modulation frequency"; audio gives
	// the coils a real waveform, which is where fine texture comes from.
	HapticL, HapticR uint8
	Lightbar         [3]uint8 // RGB
	PlayerLEDs       uint8    // 0 = off, 0x04 = player 1
	MuteLED          bool
}

// Idle returns a frame with released triggers and a dark lightbar.
func Idle() Frame { return Frame{} }

// BuildUSB serialises the frame into a 48-byte USB output report.
//
// The enable flags (bytes 1-2) mirror what the reference implementation in
// acr-dualsense used on real hardware: motors + both triggers + LEDs.
func BuildUSB(f Frame) []byte {
	b := make([]byte, USBReportLen)
	b[0] = usbReportID
	// valid_flag0 is an apply mask, so only claim what this frame actually sets.
	//
	//   bit0 COMPATIBLE_VIBRATION + bit1 HAPTICS_SELECT: "drive the actuators
	//   from motor_left/motor_right". Claiming these with zero motors hands the
	//   actuators to the HID rumble engine at amplitude 0, which makes the
	//   firmware ignore the audio-streamed haptics on channels 3/4 — the reason
	//   writing PCM to the coils appeared to do nothing.
	//
	//   bit2/bit3: left/right trigger effect blocks.
	//
	// Bits 4-7 (headphone/speaker/microphone volume, audio_control) must stay
	// clear: enabling them with the zeroed payload bytes mutes the USB audio
	// DAC entirely (verified on hardware — no sound at all until they were
	// dropped).
	flags0 := byte(0x04 | 0x08) // trigger effect blocks
	if f.RumbleL != 0 || f.RumbleR != 0 {
		flags0 |= 0x01 | 0x02
	}
	b[offFlags0] = flags0
	b[offFlags1] = 0x01 | 0x02 | 0x04 | 0x10 | 0x40
	b[offRumbleR] = f.RumbleR
	b[offRumbleL] = f.RumbleL
	if f.MuteLED {
		b[offMuteLED] = 0x01
	}
	writeTrigger(b[offR2Block:offR2Block+11], f.R2)
	writeTrigger(b[offL2Block:offL2Block+11], f.L2)
	// valid_flag2: lightbar control is always on; player-LED control is only
	// enabled when the caller actually wants a pattern, because enabling it with
	// a zero value would switch the default player indication off.
	// bit1 LIGHTBAR_SETUP_CONTROL_ENABLE keeps the lightbar under our control;
	// the remaining bit is the player-indicator enable (player LEDs are also
	// gated by valid_flag1 bit4).
	flags2 := byte(0x03)
	if f.PlayerLEDs != 0 {
		flags2 |= 0x04
	}
	b[offValid2] = flags2
	b[offBright] = 0x02
	b[offPlayerLED] = f.PlayerLEDs
	b[offLightbar] = f.Lightbar[0]
	b[offLightbar+1] = f.Lightbar[1]
	b[offLightbar+2] = f.Lightbar[2]
	return b
}

// BuildBT serialises the frame into a Bluetooth 0x31 report.
//
// The payload is the USB payload shifted by one byte (report id + one control
// byte at the front), followed by a CRC32 of the first 74 bytes.
//
// NOTE: the USB path is the verified one; the BT layout follows community
// documentation and has not been exercised on hardware here.
func BuildBT(f Frame) []byte {
	usb := BuildUSB(f)
	b := make([]byte, BTReportLen)
	b[0] = btReportID
	b[1] = 0x02 // flags required by the BM/EDR path
	copy(b[2:], usb[1:offLightbar+3])
	binary.LittleEndian.PutUint32(b[BTReportLen-4:], crc32.ChecksumIEEE(b[:BTReportLen-4]))
	return b
}

func writeTrigger(dst []byte, t Trigger) {
	if len(dst) < 11 {
		return
	}
	dst[0] = byte(t.Mode)
	copy(dst[1:11], t.P[:])
}
