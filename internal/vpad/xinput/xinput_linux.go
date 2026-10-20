//go:build linux

package xinput

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

// Linux evdev button codes for Xbox 360 controllers.
const (
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

	btnSouth = 0x130
	btnEast  = 0x131
	btnNorth = 0x134
	btnWest  = 0x133

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

// Open creates an Xbox 360 virtual gamepad via /dev/uinput.
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	if cfg.Name == "" {
		cfg.Name = "ds5vpad virtual pad"
	}
	if cfg.Vendor == 0 {
		cfg.Vendor = 0x045e
		cfg.Product = 0x028e
	}
	ufd, err := unix.Open("/dev/uinput", unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("vpad/xinput: open /dev/uinput: %w (install udev/71-ds5vpad.rules)", err)
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(ufd)
		}
	}()

	setInt := func(op, val int) {
		if err := unix.IoctlSetInt(ufd, uint(op), val); err != nil && cfg.Logger != nil {
			cfg.Logger.Printf("vpad/xinput: ioctl 0x%x(%d): %v", op, val, err)
		}
	}
	setInt(vpad.UiSetEvBit, vpad.EvKey)
	setInt(vpad.UiSetEvBit, vpad.EvAbs)
	setInt(vpad.UiSetEvBit, vpad.EvSyn)
	setInt(vpad.UiSetEvBit, vpad.EvFF)
	for _, k := range []int{btnA, btnB, btnX, btnY, btnTL, btnTR, btnSelect, btnStart,
		btnMode, btnThumbL, btnThumbR, btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt} {
		setInt(vpad.UiSetKeyBit, k)
	}
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
	for _, c := range []int{vpad.AbsX, vpad.AbsY, vpad.AbsRX, vpad.AbsRY} {
		setAbs(c, -32768, 32767)
	}
	for _, c := range []int{vpad.AbsZ, vpad.AbsRZ} {
		setAbs(c, 0, 255)
	}
	for _, c := range []int{vpad.AbsHat0X, vpad.AbsHat0Y} {
		setAbs(c, -1, 1)
	}
	if _, err := unix.Write(ufd, dev); err != nil {
		return nil, fmt.Errorf("vpad/xinput: write device descriptor: %w", err)
	}
	if err := unix.IoctlSetInt(ufd, vpad.UiDevCreate, 0); err != nil {
		return nil, fmt.Errorf("vpad/xinput: UI_DEV_CREATE: %w", err)
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
		return fmt.Errorf("vpad/xinput: EVIOCGRAB: %w", err)
	}
	defer func() {
		unix.IoctlSetInt(phys, vpad.EvIOCGrab, 0)
		unix.Close(phys)
	}()
	if p.log != nil {
		p.log("vpad/xinput: grabbed DualSense, virtual pad is %s", p.evPath)
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
			return fmt.Errorf("vpad/xinput: read: %w", err)
		}
		if outs := p.translate(buf[:n]); len(outs) > 0 {
			if _, err := unix.Write(p.ufd, outs); err != nil && err != unix.EAGAIN {
				return fmt.Errorf("vpad/xinput: inject: %w", err)
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
				p.log("vpad/xinput: ff control read stopped: %v", err)
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
				binary.LittleEndian.PutUint32(erase[4:], 0) // success
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
				}
				out = vpad.AppendEvent(out, vpad.EvAbs, mapped, val)
			}
		case vpad.EvKey:
			if mapped, ok := mapKey(code); ok {
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

func mapKey(code uint16) (uint16, bool) {
	switch code {
	case btnSouth:
		return btnA, true
	case btnEast:
		return btnB, true
	case btnWest:
		return btnX, true
	case btnNorth:
		return btnY, true
	case btnTL, btnTR, btnSelect, btnStart, btnMode, btnThumbL, btnThumbR,
		btnDpadUp, btnDpadDn, btnDpadLt, btnDpadRt:
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
