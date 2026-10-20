//go:build linux

package dinput8

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/npc1607/dualsense-c/internal/vpad"
)

// Linux evdev standard joystick button codes (0x120..0x12b),
// which DirectInput 8 enumerates as discrete Button 1..12.
const (
	btn1  = 0x120 // BTN_TRIGGER / Button 1 (DualSense Cross / ×)
	btn2  = 0x121 // BTN_THUMB   / Button 2 (DualSense Circle / ○)
	btn3  = 0x122 // BTN_THUMB2  / Button 3 (DualSense Square / 口)
	btn4  = 0x123 // BTN_TOP     / Button 4 (DualSense Triangle / △)
	btn5  = 0x124 // BTN_TOP2    / Button 5 (DualSense L1)
	btn6  = 0x125 // BTN_PINKIE  / Button 6 (DualSense R1)
	btn7  = 0x126 // BTN_BASE    / Button 7 (DualSense L2 digital)
	btn8  = 0x127 // BTN_BASE2   / Button 8 (DualSense R2 digital)
	btn9  = 0x128 // BTN_BASE3   / Button 9 (DualSense Create / Share)
	btn10 = 0x129 // BTN_BASE4   / Button 10 (DualSense Options)
	btn11 = 0x12a // BTN_BASE5   / Button 11 (DualSense L3)
	btn12 = 0x12b // BTN_BASE6   / Button 12 (DualSense R3)

	// Physical DualSense input codes from hid-playstation
	dsBtnSouth  = 0x130 // Cross / ×
	dsBtnEast   = 0x131 // Circle / ○
	dsBtnNorth  = 0x134 // Triangle / △
	dsBtnWest   = 0x133 // Square / 口
	dsBtnTL     = 0x136 // L1
	dsBtnTR     = 0x137 // R1
	dsBtnSelect = 0x13a // Create / Share
	dsBtnStart  = 0x13b // Options
	dsBtnMode   = 0x13c // PS
	dsBtnThumbL = 0x13d // L3
	dsBtnThumbR = 0x13e // R3

	btnDpadUp = 0x220
	btnDpadDn = 0x221
	btnDpadLt = 0x222
	btnDpadRt = 0x223
)

type pad struct {
	cfg    vpad.Config
	ufd    int
	evPath string
	name   string
	sink   vpad.RumbleSink
	log    func(format string, args ...any)

	mu     sync.Mutex
	closed bool
}

// Open creates a classic DirectInput 8 virtual gamepad (Logitech Dual Action 046d:c216)
// via /dev/uinput.
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	if cfg.Name == "" {
		cfg.Name = "Logitech Dual Action (DInput8)"
	}
	if cfg.Vendor == 0 {
		cfg.Vendor = 0x046d
		cfg.Product = 0xc216
	}
	ufd, err := unix.Open("/dev/uinput", unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("vpad/dinput8: open /dev/uinput: %w (install udev/71-ds5vpad.rules)", err)
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(ufd)
		}
	}()

	setInt := func(op, val int) {
		if err := unix.IoctlSetInt(ufd, uint(op), val); err != nil && cfg.Logger != nil {
			cfg.Logger.Printf("vpad/dinput8: ioctl 0x%x(%d): %v", op, val, err)
		}
	}
	setInt(vpad.UiSetEvBit, vpad.EvKey)
	setInt(vpad.UiSetEvBit, vpad.EvAbs)
	setInt(vpad.UiSetEvBit, vpad.EvSyn)
	setInt(vpad.UiSetEvBit, vpad.EvFF)

	// Discrete DirectInput buttons (1..12)
	for _, k := range []int{btn1, btn2, btn3, btn4, btn5, btn6, btn7, btn8, btn9, btn10, btn11, btn12,
		btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt} {
		setInt(vpad.UiSetKeyBit, k)
	}

	// 6 independent axes (X, Y, Z, Rx, Ry, Rz) + D-pad hat
	for _, a := range []int{vpad.AbsX, vpad.AbsY, vpad.AbsZ, vpad.AbsRX, vpad.AbsRY, vpad.AbsRZ, vpad.AbsHat0X, vpad.AbsHat0Y} {
		setInt(vpad.UiSetAbsBit, a)
	}
	setInt(vpad.UiSetFFBit, vpad.FfRumble)

	const absCnt = 0x40
	dev := make([]byte, 80+8+4+4*absCnt*4)
	copy(dev, cfg.Name)
	binary.LittleEndian.PutUint16(dev[80:], 0x03) // BUS_USB
	binary.LittleEndian.PutUint16(dev[82:], cfg.Vendor)
	binary.LittleEndian.PutUint16(dev[84:], cfg.Product)
	binary.LittleEndian.PutUint16(dev[86:], 0x0110) // version
	binary.LittleEndian.PutUint32(dev[88:], 16)     // ff_effects_max

	setAbs := func(code int, lo, hi int32) {
		base := 92 + code*4
		binary.LittleEndian.PutUint32(dev[base:], uint32(hi))          // absmax
		binary.LittleEndian.PutUint32(dev[base+absCnt*4:], uint32(lo)) // absmin
	}

	// Sticks are centred bipolar axes (-32768..32767)
	for _, c := range []int{vpad.AbsX, vpad.AbsY, vpad.AbsRX, vpad.AbsRY} {
		setAbs(c, -32768, 32767)
	}

	// Triggers in DirectInput 8 are independent unipolar pedal axes (rest 0, pressed 32767).
	// Z = L2 (Brake), RZ = R2 (Throttle).
	for _, c := range []int{vpad.AbsZ, vpad.AbsRZ} {
		setAbs(c, 0, 32767)
	}

	// D-pad hat (-1..1)
	for _, c := range []int{vpad.AbsHat0X, vpad.AbsHat0Y} {
		setAbs(c, -1, 1)
	}

	if _, err := unix.Write(ufd, dev); err != nil {
		return nil, fmt.Errorf("vpad/dinput8: write device descriptor: %w", err)
	}
	if err := unix.IoctlSetInt(ufd, vpad.UiDevCreate, 0); err != nil {
		return nil, fmt.Errorf("vpad/dinput8: UI_DEV_CREATE: %w", err)
	}

	evPath, err := vpad.EventNode(ufd)
	if err != nil {
		unix.IoctlSetInt(ufd, vpad.UiDevDestry, 0)
		return nil, err
	}
	ok = true
	p := &pad{cfg: cfg, ufd: ufd, evPath: evPath, name: cfg.Name, sink: sink}
	if cfg.Logger != nil {
		p.log = cfg.Logger.Printf
	}
	return p, nil
}

func (p *pad) Name() string { return p.name }

func (p *pad) Run(ctx context.Context) error {
	phys, err := vpad.FindPhysical(p.cfg.GrabPath)
	if err != nil {
		return err
	}
	if err := unix.IoctlSetInt(phys, vpad.EvIOCGrab, 1); err != nil {
		unix.Close(phys)
		return fmt.Errorf("vpad/dinput8: EVIOCGRAB: %w", err)
	}
	defer func() {
		unix.IoctlSetInt(phys, vpad.EvIOCGrab, 0)
		unix.Close(phys)
	}()
	if p.log != nil {
		p.log("vpad/dinput8: grabbed DualSense, virtual pad is %s", p.evPath)
	}

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
			return fmt.Errorf("vpad/dinput8: read: %w", err)
		}
		if outs := p.translate(buf[:n]); len(outs) > 0 {
			if _, err := unix.Write(p.ufd, outs); err != nil && err != unix.EAGAIN {
				return fmt.Errorf("vpad/dinput8: inject: %w", err)
			}
		}
	}
}

func (p *pad) serveFF(ctx context.Context) {
	buf := make([]byte, 24*16)
	upload := make([]byte, vpad.RumbleEventBytes)
	erase := make([]byte, 12)
	for ctx.Err() == nil {
		n, err := unix.Read(p.ufd, buf)
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			if p.log != nil {
				p.log("vpad/dinput8: ff control read stopped: %v", err)
			}
			return
		}
		for off := 0; off+24 <= n; off += 24 {
			typ := binary.LittleEndian.Uint16(buf[off+16:])
			code := binary.LittleEndian.Uint16(buf[off+18:])
			val := binary.LittleEndian.Uint32(buf[off+20:])
			if typ != vpad.EvUInput {
				continue
			}
			switch code {
			case vpad.UiFFUpload:
				binary.LittleEndian.PutUint32(upload[0:], val)
				if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd),
					vpad.UiBeginFFUpload, uintptr(unsafe.Pointer(&upload[0]))); errno != 0 {
					continue
				}
				p.reportUpload(upload)
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), vpad.UiEndFFUpload,
					uintptr(unsafe.Pointer(&upload[0])))
			case vpad.UiFFErase:
				binary.LittleEndian.PutUint32(erase[0:], val)
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), vpad.UiBeginFFErase,
					uintptr(unsafe.Pointer(&erase[0])))
				binary.LittleEndian.PutUint32(erase[4:], 0)
				unix.Syscall(unix.SYS_IOCTL, uintptr(p.ufd), vpad.UiEndFFErase,
					uintptr(unsafe.Pointer(&erase[0])))
			}
		}
	}
}

func (p *pad) reportUpload(b []byte) {
	const effOff = 8
	typ := binary.LittleEndian.Uint16(b[effOff:])
	if typ != vpad.FfRumble {
		return
	}
	strong := binary.LittleEndian.Uint16(b[effOff+12:])
	weak := binary.LittleEndian.Uint16(b[effOff+14:])
	if p.sink == nil {
		return
	}
	p.sink(vpad.Rumble{
		Left:  vpad.Scale16(strong),
		Right: vpad.Scale16(weak),
		At:    time.Now(),
	})
}

func (p *pad) translate(buf []byte) []byte {
	out := make([]byte, 0, len(buf))
	for off := 0; off+24 <= len(buf); off += 24 {
		typ := binary.LittleEndian.Uint16(buf[off+16:])
		code := binary.LittleEndian.Uint16(buf[off+18:])
		val := int32(binary.LittleEndian.Uint32(buf[off+20:]))

		switch typ {
		case vpad.EvSyn:
			out = vpad.AppendEvent(out, vpad.EvSyn, 0, 0)
		case vpad.EvAbs:
			if mapped, ok := mapAbs(code); ok {
				if code == vpad.AbsX || code == vpad.AbsY || code == vpad.AbsRX || code == vpad.AbsRY {
					val = vpad.ScaleStick(val)
				} else if code == vpad.AbsZ || code == vpad.AbsRZ {
					// DirectInput 8 unipolar pedal axis: 0..255 -> 0..32767
					val = (vpad.ClampByte(val) * 32767) / 255
				}
				out = vpad.AppendEvent(out, vpad.EvAbs, mapped, val)
			}
		case vpad.EvKey:
			if mapped, ok := mapKeyDInput8(code); ok {
				out = vpad.AppendEvent(out, vpad.EvKey, mapped, val)
			}
		}
	}
	return out
}

func mapAbs(code uint16) (uint16, bool) {
	switch code {
	case vpad.AbsX, vpad.AbsY, vpad.AbsRX, vpad.AbsRY, vpad.AbsZ, vpad.AbsRZ, vpad.AbsHat0X, vpad.AbsHat0Y:
		return code, true
	}
	return 0, false
}

// mapKeyDInput8 maps DualSense buttons directly to standard DirectInput 8 buttons (1..12).
func mapKeyDInput8(code uint16) (uint16, bool) {
	switch code {
	case dsBtnSouth: // Cross / × -> Button 1
		return btn1, true
	case dsBtnEast: // Circle / ○ -> Button 2
		return btn2, true
	case dsBtnWest: // Square / 口 -> Button 3
		return btn3, true
	case dsBtnNorth: // Triangle / △ -> Button 4
		return btn4, true
	case dsBtnTL: // L1 -> Button 5
		return btn5, true
	case dsBtnTR: // R1 -> Button 6
		return btn6, true
	case dsBtnSelect: // Create / Share -> Button 9
		return btn9, true
	case dsBtnStart: // Options -> Button 10
		return btn10, true
	case dsBtnThumbL: // L3 -> Button 11
		return btn11, true
	case dsBtnThumbR: // R3 -> Button 12
		return btn12, true
	case btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt:
		return code, true
	}
	return 0, false
}

func (p *pad) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	unix.IoctlSetInt(p.ufd, vpad.UiDevDestry, 0)
	return unix.Close(p.ufd)
}
