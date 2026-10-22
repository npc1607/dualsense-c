//go:build windows

package dinput8

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/npc1607/dualsense-c/internal/aim"
	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/input"
	"github.com/npc1607/dualsense-c/internal/vpad"
)

const (
	vigemErrorNone         = 0x20000000
	vigemErrorBusNotFound  = 0xE0000001
	vigemErrorNoFreeSlot   = 0xE0000002
	vigemErrorAccessFailed = 0xE0000009
)

var (
	modViGEm = windows.NewLazyDLL("ViGEmClient.dll")

	procVigemAlloc                         = modViGEm.NewProc("vigem_alloc")
	procVigemFree                          = modViGEm.NewProc("vigem_free")
	procVigemConnect                       = modViGEm.NewProc("vigem_connect")
	procVigemDisconnect                    = modViGEm.NewProc("vigem_disconnect")
	procVigemTargetDS4Alloc                = modViGEm.NewProc("vigem_target_ds4_alloc")
	procVigemTargetFree                    = modViGEm.NewProc("vigem_target_free")
	procVigemTargetSetVid                  = modViGEm.NewProc("vigem_target_set_vid")
	procVigemTargetSetPid                  = modViGEm.NewProc("vigem_target_set_pid")
	procVigemTargetAdd                     = modViGEm.NewProc("vigem_target_add")
	procVigemTargetRemove                  = modViGEm.NewProc("vigem_target_remove")
	procVigemTargetDS4Update               = modViGEm.NewProc("vigem_target_ds4_update")
	procVigemTargetDS4RegisterNotification = modViGEm.NewProc("vigem_target_ds4_register_notification")
	procVigemTargetDS4Unregister           = modViGEm.NewProc("vigem_target_ds4_unregister_notification")
)

var (
	cbOnce    sync.Once
	ds4Notify uintptr

	padsMu     sync.RWMutex
	activePads = map[uintptr]*winPad{}
)

func getNotifyCallback() uintptr {
	cbOnce.Do(func() {
		ds4Notify = syscall.NewCallback(func(client, target, largeMotor, smallMotor, lightbarColor, userData uintptr) uintptr {
			padsMu.RLock()
			p := activePads[target]
			padsMu.RUnlock()
			if p != nil && p.sink != nil {
				p.sink(vpad.Rumble{
					Left:  uint8(largeMotor),
					Right: uint8(smallMotor),
					At:    time.Now(),
				})
			}
			return 0
		})
	})
	return ds4Notify
}

func vigemErr(op string, code uintptr) error {
	c := uint32(code)
	switch c {
	case vigemErrorBusNotFound:
		return fmt.Errorf("vpad/dinput8: %s: ViGEmBus driver not found (install ViGEmBus)", op)
	case vigemErrorNoFreeSlot:
		return fmt.Errorf("vpad/dinput8: %s: no free ViGEmBus slot", op)
	case vigemErrorAccessFailed:
		return fmt.Errorf("vpad/dinput8: %s: ViGEmBus access failed", op)
	default:
		return fmt.Errorf("vpad/dinput8: %s failed (VIGEM_ERROR 0x%08x)", op, c)
	}
}

type winPad struct {
	cfg     vpad.Config
	name    string
	client  uintptr
	target  uintptr
	sink    vpad.RumbleSink
	log     func(format string, args ...any)
	aimProc *aim.Processor

	mu     sync.Mutex
	closed bool
}

// Open creates a virtual DualShock 4 controller on Windows via ViGEmBus and
// registers a native force-feedback notification callback.
//
// Windows natively recognizes the ViGEm DS4 target via hidclass.sys/hidusb.sys
// as a 6-axis, 14-button DirectInput 8 gamepad with independent pedal axes (L2/R2).
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	if cfg.Name == "" {
		cfg = vpad.DInput8Config()
	}
	if err := modViGEm.Load(); err != nil {
		return nil, fmt.Errorf("vpad/dinput8: cannot load ViGEmClient.dll (install ViGEmBus and place ViGEmClient.dll in PATH or next to the binary): %w", err)
	}

	client, _, _ := procVigemAlloc.Call()
	if client == 0 {
		return nil, fmt.Errorf("vpad/dinput8: vigem_alloc returned NULL")
	}

	if ret, _, _ := procVigemConnect.Call(client); uint32(ret) != vigemErrorNone {
		procVigemFree.Call(client)
		return nil, vigemErr("vigem_connect", ret)
	}

	target, _, _ := procVigemTargetDS4Alloc.Call()
	if target == 0 {
		procVigemDisconnect.Call(client)
		procVigemFree.Call(client)
		return nil, fmt.Errorf("vpad/dinput8: vigem_target_ds4_alloc returned NULL")
	}

	// vpad.DInput8Config() defaults to Logitech Dual Action (0x046d:0xc216) for Linux evdev.
	// On Windows, ViGEm DS4 must retain Sony identity (0x054C:0x05C4) so Windows binds
	// hidclass.sys/hidusb.sys. We only override VID/PID if explicitly set to something
	// other than the default Logitech IDs.
	if cfg.Vendor != 0 && cfg.Vendor != 0x046d {
		procVigemTargetSetVid.Call(target, uintptr(cfg.Vendor))
	}
	if cfg.Product != 0 && cfg.Product != 0xc216 {
		procVigemTargetSetPid.Call(target, uintptr(cfg.Product))
	}

	if ret, _, _ := procVigemTargetAdd.Call(client, target); uint32(ret) != vigemErrorNone {
		procVigemTargetFree.Call(target)
		procVigemDisconnect.Call(client)
		procVigemFree.Call(client)
		return nil, vigemErr("vigem_target_add", ret)
	}

	p := &winPad{
		cfg:     cfg,
		name:    cfg.Name,
		client:  client,
		target:  target,
		sink:    sink,
		aimProc: vpad.NewAimProcessor(cfg.Aim),
	}
	if cfg.Logger != nil {
		p.log = cfg.Logger.Printf
	}

	padsMu.Lock()
	activePads[target] = p
	padsMu.Unlock()

	if ret, _, _ := procVigemTargetDS4RegisterNotification.Call(
		client,
		target,
		getNotifyCallback(),
		0,
	); uint32(ret) != vigemErrorNone {
		_ = p.Close()
		return nil, vigemErr("vigem_target_ds4_register_notification", ret)
	}

	return p, nil
}

func (p *winPad) Name() string { return p.name }

// Run reads HID input reports from the physical DualSense controller, maps
// them to DS4_REPORT, and updates the ViGEm virtual DS4 pad until ctx is
// cancelled.
func (p *winPad) Run(ctx context.Context) error {
	phys, err := p.openPhysical()
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = phys.Close()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		_ = phys.Close()
	}()

	if p.log != nil {
		p.log("vpad/dinput8: reading %s, forwarding to ViGEm DS4 pad (%s)", phys.Info(), p.name)
	}

	buf := make([]byte, 128)
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := phys.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("vpad/dinput8: read %s: %w", phys.Info().Path, err)
		}
		st, ok := input.Parse(buf[:n])
		if !ok {
			continue
		}
		if p.aimProc != nil {
			outRX, outRY := p.aimProc.Process(aim.InputFrame{
				RX:          st.RX,
				RY:          st.RY,
				GyroYaw:     st.GyroY,
				GyroPitch:   st.GyroX,
				GyroRoll:    st.GyroZ,
				L2:          st.L2,
				R2:          st.R2,
				TouchActive: st.TouchActive,
				At:          time.Now(),
			})
			st.RX = outRX
			st.RY = outRY
		}
		rep := encodeDS4Report(st, p.cfg.Deadzone)
		if err := p.updateReport(rep); err != nil {
			return err
		}
	}
}

func (p *winPad) openPhysical() (device.Device, error) {
	if p.cfg.GrabPath != "" {
		return device.OpenPath(p.cfg.GrabPath)
	}
	info, err := device.Find()
	if err != nil {
		return nil, fmt.Errorf("vpad/dinput8: %w", err)
	}
	return device.Open(info)
}

func (p *winPad) updateReport(rep [10]byte) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	var ret uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		// Microsoft x64 ABI passes 10-byte struct by pointer in R8
		ret, _, _ = procVigemTargetDS4Update.Call(
			p.client,
			p.target,
			uintptr(unsafe.Pointer(&rep[0])),
		)
	} else {
		// 32-bit fallback stack words
		w0 := uintptr(binary.LittleEndian.Uint32(rep[0:4]))
		w1 := uintptr(binary.LittleEndian.Uint32(rep[4:8]))
		w2 := uintptr(binary.LittleEndian.Uint16(rep[8:10]))
		ret, _, _ = procVigemTargetDS4Update.Call(p.client, p.target, w0, w1, w2)
	}
	p.mu.Unlock()

	if uint32(ret) != vigemErrorNone {
		return vigemErr("vigem_target_ds4_update", ret)
	}
	return nil
}

// Close unregisters notifications, unplugs the virtual DS4 target, and
// disconnects from ViGEmBus.
func (p *winPad) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	padsMu.Lock()
	delete(activePads, p.target)
	padsMu.Unlock()

	if p.target != 0 && p.client != 0 {
		_, _, _ = procVigemTargetDS4Unregister.Call(p.target)
		_, _, _ = procVigemTargetRemove.Call(p.client, p.target)
		_, _, _ = procVigemTargetFree.Call(p.target)
		p.target = 0
	}
	if p.client != 0 {
		_, _, _ = procVigemDisconnect.Call(p.client)
		_, _, _ = procVigemFree.Call(p.client)
		p.client = 0
	}
	return nil
}

// DeviceExists reports whether a virtual pad with the given name is currently
// active in this process.
func DeviceExists(name string) bool {
	padsMu.RLock()
	defer padsMu.RUnlock()
	for _, p := range activePads {
		if p.name == name {
			return true
		}
	}
	return false
}
