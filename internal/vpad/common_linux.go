//go:build linux

package vpad

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux uinput ioctl numbers (from linux/uinput.h).
const (
	UiSetEvBit  = 0x40045564
	UiSetKeyBit = 0x40045565
	UiSetAbsBit = 0x40045567
	UiSetFFBit  = 0x4004556b
	UiDevCreate = 0x5501
	UiDevDestry = 0x5502

	UiBeginFFUpload = 0xC06855C8
	UiEndFFUpload   = 0x406855C9
	UiBeginFFErase  = 0xC00C55CA
	UiEndFFErase    = 0x400C55CB

	EvFF     = 0x15
	EvUInput = 0x0101

	// EvIOCGrab is _IOW('E', 0x90, int).
	EvIOCGrab = 0x40044590

	UiFFUpload = 1
	UiFFErase  = 2
	FfRumble   = 0x50

	EvSyn = 0x00
	EvKey = 0x01
	EvAbs = 0x03

	AbsX     = 0x00
	AbsY     = 0x01
	AbsZ     = 0x02
	AbsRX    = 0x03
	AbsRY    = 0x04
	AbsRZ    = 0x05
	AbsHat0X = 0x10
	AbsHat0Y = 0x11
)

// RumbleEventBytes is sizeof(struct uinput_ff_upload).
const RumbleEventBytes = 8 + 48 + 48

// ScaleStick maps a physical DualSense raw stick axis (0..255, center ~128)
// to the standard full-range signed axis (-32768..32767, center 0).
func ScaleStick(val int32) int32 {
	v := (val - 128) * 32767 / 127
	if v < -32768 {
		return -32768
	}
	if v > 32767 {
		return 32767
	}
	return v
}

// ClampByte keeps a raw evdev axis value inside the 0..255 trigger range.
func ClampByte(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// Scale16 compresses a 16-bit rumble magnitude (0..65535) into an 8-bit motor value (0..255).
func Scale16(v uint16) uint8 {
	return uint8((uint32(v)*255 + 32767) / 65535)
}

// AppendEvent appends a 24-byte struct input_event into dst.
func AppendEvent(dst []byte, typ, code uint16, val int32) []byte {
	var ev [24]byte
	binary.LittleEndian.PutUint16(ev[16:], typ)
	binary.LittleEndian.PutUint16(ev[18:], code)
	binary.LittleEndian.PutUint32(ev[20:], uint32(val))
	return append(dst, ev[:]...)
}

// FindPhysical locates (and opens) the DualSense gamepad evdev node.
func FindPhysical(grabPath string) (int, error) {
	if grabPath != "" {
		fd, err := unix.Open(grabPath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return -1, fmt.Errorf("vpad: open %s: %w", grabPath, err)
		}
		return fd, nil
	}
	matches, _ := filepath.Glob("/dev/input/event*")
	for _, path := range matches {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		name := evdevName(fd)
		if strings.Contains(name, "DualSense") &&
			!strings.Contains(name, "Motion") &&
			!strings.Contains(name, "Touchpad") &&
			!strings.Contains(name, "Headset") {
			return fd, nil
		}
		unix.Close(fd)
	}
	return -1, fmt.Errorf("vpad: no DualSense evdev gamepad found (see /proc/bus/input/devices)")
}

func evdevName(fd int) string {
	buf := make([]byte, 128)
	req := uintptr(0x80000000 | (len(buf) << 16) | ('E' << 8) | 0x06) // EVIOCGNAME
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return ""
	}
	return string(buf[:Cstrlen(buf)])
}

// EventNode resolves the /dev/input/eventN that belongs to the uinput device.
func EventNode(ufd int) (string, error) {
	buf := make([]byte, 64)
	req := uintptr(0x80000000 | (len(buf) << 16) | ('U' << 8) | 44) // UI_GET_SYSNAME
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(ufd), req, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return "", fmt.Errorf("vpad: UI_GET_SYSNAME: %w", errno)
	}
	sys := string(buf[:Cstrlen(buf)])
	matches, _ := filepath.Glob("/sys/devices/virtual/input/" + sys + "/event*")
	if len(matches) == 0 {
		return "", fmt.Errorf("vpad: no event node for %s", sys)
	}
	return "/dev/input/" + filepath.Base(matches[0]), nil
}

// Cstrlen finds the index of the first NUL byte.
func Cstrlen(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return len(b)
}
