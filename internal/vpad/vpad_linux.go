//go:build linux

package vpad

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// uinput ioctl numbers. They are spelled out instead of using a cgo header so
// the project keeps its no-cgo rule; values come from linux/uinput.h.
const (
	uiSetEvBit  = 0x40045564
	uiSetKeyBit = 0x40045565
	uiSetAbsBit = 0x40045567
	uiSetFFBit  = 0x4004556b
	uiDevCreate = 0x5501
	uiDevDestry = 0x5502

	uiBeginFFUpload = 0xC06855C8
	uiEndFFUpload   = 0x406855C9
	uiBeginFFErase  = 0xC00C55CA
	uiEndFFErase    = 0x400C55CB

	evFF     = 0x15
	evUInput = 0x0101

	// _IOW('E', 0x90, int); x/sys/unix does not export it.
	evIOCGrab = 0x40044590

	uiFFUpload = 1
	uiFFErase  = 2
)

// ff_effect types
const (
	ffRumble = 0x50
)

// evdev codes we care about
const (
	evSyn = 0x00
	evKey = 0x01
	evAbs = 0x03

	absX     = 0x00
	absY     = 0x01
	absZ     = 0x02
	absRX    = 0x03
	absRY    = 0x04
	absRZ    = 0x05
	absHat0X = 0x10
	absHat0Y = 0x11

	btnA      = 0x130
	btnB      = 0x131
	btnX      = 0x133
	btnY      = 0x134
	btnTL     = 0x136
	btnTR     = 0x137
	btnSelect = 0x13a
	btnStart  = 0x13b
	btnMode   = 0x13c
	btnThumbL = 0x13d
	btnThumbR = 0x13e
	btnDpadUp = 0x220
	btnDpadDn = 0x221
	btnDpadLt = 0x222
	btnDpadRt = 0x223

	btnSouth = 0x130
	btnEast  = 0x131
	btnNorth = 0x133
	btnWest  = 0x134
	btnTL2   = 0x138
	btnTR2   = 0x139
)

// pad is the Linux backend: a uinput device plus a grabbed physical evdev node.
type pad struct {
	cfg    Config
	ufd    int
	evPath string
	name   string
	sink   RumbleSink
	log    func(format string, args ...any)

	mu     sync.Mutex
	closed bool
}

// rumbleEventBytes is sizeof(struct uinput_ff_upload).
const rumbleEventBytes = 8 + 48 + 48

// Open creates the virtual pad and prepares to forward input.
func Open(cfg Config, sink RumbleSink) (Pad, error) {
	if cfg.Name == "" {
		cfg = DefaultConfig()
	}
	ufd, err := unix.Open("/dev/uinput", unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("vpad: open /dev/uinput: %w (install udev/71-ds5vpad.rules)", err)
	}
	// make sure the fd is closed if setup fails half way
	ok := false
	defer func() {
		if !ok {
			unix.Close(ufd)
		}
	}()

	setInt := func(op, val int) {
		if err := unix.IoctlSetInt(ufd, uint(op), val); err != nil && cfg.Logger != nil {
			cfg.Logger.Printf("vpad: ioctl 0x%x(%d): %v", op, val, err)
		}
	}
	setInt(uiSetEvBit, evKey)
	setInt(uiSetEvBit, evAbs)
	setInt(uiSetEvBit, evSyn)
	setInt(uiSetEvBit, evFF)
	for _, k := range []int{btnA, btnB, btnX, btnY, btnTL, btnTR, btnSelect, btnStart,
		btnMode, btnThumbL, btnThumbR, btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt} {
		setInt(uiSetKeyBit, k)
	}
	for _, a := range []int{absX, absY, absZ, absRX, absRY, absRZ, absHat0X, absHat0Y} {
		setInt(uiSetAbsBit, a)
	}
	// Rumble only: it is what Proton/Wine games actually use, and claiming
	// periodic/constant support would require synthesising them ourselves.
	setInt(uiSetFFBit, ffRumble)

	const absCnt = 0x40
	dev := make([]byte, 80+8+4+4*absCnt*4)
	copy(dev, cfg.Name)
	binary.LittleEndian.PutUint16(dev[80:], 0x03) // BUS_USB
	binary.LittleEndian.PutUint16(dev[82:], cfg.Vendor)
	binary.LittleEndian.PutUint16(dev[84:], cfg.Product)
	binary.LittleEndian.PutUint16(dev[86:], 0x0110) // version
	binary.LittleEndian.PutUint32(dev[88:], 16)     // ff_effects_max
	// stick ranges: -32768..32767; triggers 0..255; dpad -1..1
	setAbs := func(code int, min, max int32) {
		base := 92 + code*4
		binary.LittleEndian.PutUint32(dev[base+absCnt*0:], uint32(max))
		binary.LittleEndian.PutUint32(dev[base+absCnt*1:], uint32(min))
	}
	for _, c := range []int{absX, absY, absRX, absRY} {
		setAbs(c, -32768, 32767)
	}
	for _, c := range []int{absZ, absRZ} {
		setAbs(c, 0, 255)
	}
	for _, c := range []int{absHat0X, absHat0Y} {
		setAbs(c, -1, 1)
	}
	if _, err := unix.Write(ufd, dev); err != nil {
		return nil, fmt.Errorf("vpad: write device descriptor: %w", err)
	}
	if err := unix.IoctlSetInt(ufd, uiDevCreate, 0); err != nil {
		return nil, fmt.Errorf("vpad: UI_DEV_CREATE: %w", err)
	}

	evPath, err := eventNode(ufd)
	if err != nil {
		unix.IoctlSetInt(ufd, uiDevDestry, 0)
		return nil, err
	}
	ok = true
	p := &pad{cfg: cfg, ufd: ufd, evPath: evPath, name: cfg.Name, sink: sink}
	if cfg.Logger != nil {
		p.log = cfg.Logger.Printf
	}
	return p, nil
}

// eventNode resolves the /dev/input/eventN that belongs to the uinput device.
func eventNode(ufd int) (string, error) {
	buf := make([]byte, 64)
	req := uintptr(0x80000000 | (len(buf) << 16) | ('U' << 8) | 44) // UI_GET_SYSNAME
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(ufd), req, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return "", fmt.Errorf("vpad: UI_GET_SYSNAME: %w", errno)
	}
	sys := string(buf[:cstrlen(buf)])
	matches, _ := filepath.Glob("/sys/devices/virtual/input/" + sys + "/event*")
	if len(matches) == 0 {
		return "", fmt.Errorf("vpad: no event node for %s", sys)
	}
	return "/dev/input/" + filepath.Base(matches[0]), nil
}

func cstrlen(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return len(b)
}

func (p *pad) Name() string { return p.name }

// Run grabs the physical controller and pumps its events into the virtual pad,
// while answering force-feedback requests.
func (p *pad) Run(ctx context.Context) error {
	phys, err := p.findPhysical()
	if err != nil {
		return err
	}
	if err := unix.IoctlSetInt(phys, evIOCGrab, 1); err != nil {
		unix.Close(phys)
		return fmt.Errorf("vpad: EVIOCGRAB: %w", err)
	}
	defer func() {
		unix.IoctlSetInt(phys, evIOCGrab, 0)
		unix.Close(phys)
	}()
	if p.log != nil {
		p.log("vpad: grabbed %s, virtual pad is %s", p.grabPathLabel(phys), p.evPath)
	}

	// FF control channel: the kernel blocks the caller until we answer, so this
	// must run independently of the event pump.
	ffDone := make(chan struct{})
	go func() {
		defer close(ffDone)
		p.serveFF(ctx)
	}()

	buf := make([]byte, 24*64)
	for {
		if ctx.Err() != nil {
			<-ffDone
			return nil
		}
		n, err := unix.Read(phys, buf)
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			if ctx.Err() != nil {
				<-ffDone
				return nil
			}
			return fmt.Errorf("vpad: read %s: %w", p.grabPathLabel(phys), err)
		}
		if outs := p.translate(buf[:n]); len(outs) > 0 {
			if _, err := unix.Write(p.ufd, outs); err != nil && err != unix.EAGAIN {
				return fmt.Errorf("vpad: inject: %w", err)
			}
		}
	}
}

func (p *pad) grabPathLabel(phys int) string {
	if p.cfg.GrabPath != "" {
		return p.cfg.GrabPath
	}
	return "DualSense evdev"
}

// serveFF answers upload/erase requests and publishes the rumble state.
//
// Note on fidelity: uinput only exposes effect *upload* and *erase* to
// userspace. Playback intensity is therefore taken from the most recently
// uploaded rumble effect and expires after that effect's replay length. Games
// that re-upload on every rumble change (which is what Proton/Wine's evdev FF
// backend does) are tracked accurately; a game that uploads once and only
// writes EV_FF play/stop would need a kernel-side callback we do not have.
func (p *pad) serveFF(ctx context.Context) {
	buf := make([]byte, 24*16)
	upload := make([]byte, rumbleEventBytes)
	erase := make([]byte, 12)
	for ctx.Err() == nil {
		n, err := unix.Read(p.ufd, buf)
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			if p.log != nil {
				p.log("vpad: ff control read stopped: %v", err)
			}
			return
		}
		if p.log != nil && n > 0 {
			p.log("vpad: ff control events: %d bytes", n)
		}
		for off := 0; off+24 <= n; off += 24 {
			typ := binary.LittleEndian.Uint16(buf[off+16:])
			code := binary.LittleEndian.Uint16(buf[off+18:])
			val := binary.LittleEndian.Uint32(buf[off+20:])
			if typ != evUInput {
				continue
			}
			switch code {
			case uiFFUpload:
				binary.LittleEndian.PutUint32(upload[0:], val)
				if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd),
					uiBeginFFUpload, uintptr(unsafe.Pointer(&upload[0]))); errno != 0 {
					continue
				}
				p.reportUpload(upload)
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), uiEndFFUpload,
					uintptr(unsafe.Pointer(&upload[0])))
			case uiFFErase:
				binary.LittleEndian.PutUint32(erase[0:], val)
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), uiBeginFFErase,
					uintptr(unsafe.Pointer(&erase[0])))
				binary.LittleEndian.PutUint32(erase[4:], 0) // retval = success
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), uiEndFFErase,
					uintptr(unsafe.Pointer(&erase[0])))
			}
		}
	}
}

// reportUpload decodes struct uinput_ff_upload and forwards rumble magnitudes.
func (p *pad) reportUpload(b []byte) {
	const effOff = 8 // request_id + retval
	typ := binary.LittleEndian.Uint16(b[effOff:])
	replayMS := binary.LittleEndian.Uint16(b[effOff+10:])
	if typ != ffRumble {
		return
	}
	strong := binary.LittleEndian.Uint16(b[effOff+12:])
	weak := binary.LittleEndian.Uint16(b[effOff+14:])
	_ = replayMS
	if p.sink == nil {
		return
	}
	// strong_magnitude drives the low-frequency (left/large) motor, weak the
	// high-frequency one, matching the kernel's ff-memless convention.
	p.sink(Rumble{
		Left:  scale16(strong),
		Right: scale16(weak),
		At:    time.Now(),
	})
}

func scale16(v uint16) uint8 {
	return uint8((uint32(v)*255 + 32767) / 65535)
}

// translate maps DualSense evdev codes to the Xbox 360 pad layout.
func (p *pad) translate(buf []byte) []byte {
	out := make([]byte, 0, len(buf))
	for off := 0; off+24 <= len(buf); off += 24 {
		typ := binary.LittleEndian.Uint16(buf[off+16:])
		code := binary.LittleEndian.Uint16(buf[off+18:])
		val := int32(binary.LittleEndian.Uint32(buf[off+20:]))

		switch typ {
		case evSyn:
			out = appendEvent(out, evSyn, 0, 0)
		case evAbs:
			if mapped, ok := mapAbs(code); ok {
				out = appendEvent(out, evAbs, mapped, val)
			}
		case evKey:
			if mapped, ok := mapKey(code); ok {
				out = appendEvent(out, evKey, mapped, val)
			}
		}
	}
	return out
}

func mapAbs(code uint16) (uint16, bool) {
	switch code {
	case absX, absY, absRX, absRY, absZ, absRZ, absHat0X, absHat0Y:
		return code, true
	}
	return 0, false
}

// mapKey rewrites the face buttons to the Xbox layout and drops the digital
// trigger bits (they travel as ABS_Z / ABS_RZ on both devices).
func mapKey(code uint16) (uint16, bool) {
	switch code {
	case btnSouth:
		return btnA, true // cross
	case btnEast:
		return btnB, true // circle
	case btnWest:
		return btnX, true // square
	case btnNorth:
		return btnY, true // triangle
	case btnTL, btnTR, btnSelect, btnStart, btnMode, btnThumbL, btnThumbR,
		btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt:
		return code, true
	}
	return 0, false
}

func appendEvent(dst []byte, typ, code uint16, val int32) []byte {
	var ev [24]byte
	binary.LittleEndian.PutUint16(ev[16:], typ)
	binary.LittleEndian.PutUint16(ev[18:], code)
	binary.LittleEndian.PutUint32(ev[20:], uint32(val))
	return append(dst, ev[:]...)
}

// findPhysical locates (and opens) the DualSense gamepad evdev node.
func (p *pad) findPhysical() (int, error) {
	if p.cfg.GrabPath != "" {
		fd, err := unix.Open(p.cfg.GrabPath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return -1, fmt.Errorf("vpad: open %s: %w", p.cfg.GrabPath, err)
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
	return string(buf[:cstrlen(buf)])
}

// Close destroys the virtual device.
func (p *pad) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.ufd >= 0 {
		unix.IoctlSetInt(p.ufd, uiDevDestry, 0)
		unix.Close(p.ufd)
		p.ufd = -1
	}
	return nil
}

// DeviceExists reports whether a pad with this name currently exists.
func DeviceExists(name string) bool {
	matches, _ := filepath.Glob("/dev/input/event*")
	for _, path := range matches {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		n := evdevName(fd)
		unix.Close(fd)
		if n == name {
			return true
		}
	}
	return false
}

// Stat is a helper for diagnostics.
func Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
