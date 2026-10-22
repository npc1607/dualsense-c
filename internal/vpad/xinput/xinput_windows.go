//go:build windows

package xinput

import (
	"github.com/npc1607/dualsense-c/internal/vpad"
)

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
)

const (
	vigemErrorNone         = 0x20000000
	vigemErrorBusNotFound  = 0xE0000001
	vigemErrorNoFreeSlot   = 0xE0000002
	vigemErrorAccessFailed = 0xE0000009
)

var (
	modViGEm = windows.NewLazyDLL("ViGEmClient.dll")

	procVigemAlloc                          = modViGEm.NewProc("vigem_alloc")
	procVigemFree                           = modViGEm.NewProc("vigem_free")
	procVigemConnect                        = modViGEm.NewProc("vigem_connect")
	procVigemDisconnect                     = modViGEm.NewProc("vigem_disconnect")
	procVigemTargetX360Alloc                = modViGEm.NewProc("vigem_target_x360_alloc")
	procVigemTargetFree                     = modViGEm.NewProc("vigem_target_free")
	procVigemTargetSetVid                   = modViGEm.NewProc("vigem_target_set_vid")
	procVigemTargetSetPid                   = modViGEm.NewProc("vigem_target_set_pid")
	procVigemTargetAdd                      = modViGEm.NewProc("vigem_target_add")
	procVigemTargetRemove                   = modViGEm.NewProc("vigem_target_remove")
	procVigemTargetX360Update               = modViGEm.NewProc("vigem_target_x360_update")
	procVigemTargetX360RegisterNotification = modViGEm.NewProc("vigem_target_x360_register_notification")
	procVigemTargetX360Unregister           = modViGEm.NewProc("vigem_target_x360_unregister_notification")
)

var (
	cbOnce     sync.Once
	x360Notify uintptr

	padsMu     sync.RWMutex
	activePads = map[uintptr]*winPad{}
)

func getNotifyCallback() uintptr {
	cbOnce.Do(func() {
		x360Notify = syscall.NewCallback(func(client, target, largeMotor, smallMotor, ledNumber, userData uintptr) uintptr {
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
	return x360Notify
}

func vigemErr(op string, code uintptr) error {
	c := uint32(code)
	switch c {
	case vigemErrorBusNotFound:
		return fmt.Errorf("vpad: %s: ViGEmBus driver not found (install ViGEmBus)", op)
	case vigemErrorNoFreeSlot:
		return fmt.Errorf("vpad: %s: no free ViGEmBus slot", op)
	case vigemErrorAccessFailed:
		return fmt.Errorf("vpad: %s: ViGEmBus access failed", op)
	default:
		return fmt.Errorf("vpad: %s failed (VIGEM_ERROR 0x%08x)", op, c)
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

// Open creates a virtual Xbox 360 controller on Windows via ViGEmBus and
// registers a native force-feedback notification callback.
func Open(cfg vpad.Config, sink vpad.RumbleSink) (vpad.Pad, error) {
	if cfg.Name == "" {
		cfg = vpad.DefaultConfig()
	}
	if err := modViGEm.Load(); err != nil {
		return nil, fmt.Errorf("vpad: cannot load ViGEmClient.dll (install ViGEmBus and place ViGEmClient.dll in PATH or next to the binary): %w", err)
	}

	client, _, _ := procVigemAlloc.Call()
	if client == 0 {
		return nil, fmt.Errorf("vpad: vigem_alloc returned NULL")
	}

	if ret, _, _ := procVigemConnect.Call(client); uint32(ret) != vigemErrorNone {
		procVigemFree.Call(client)
		return nil, vigemErr("vigem_connect", ret)
	}

	target, _, _ := procVigemTargetX360Alloc.Call()
	if target == 0 {
		procVigemDisconnect.Call(client)
		procVigemFree.Call(client)
		return nil, fmt.Errorf("vpad: vigem_target_x360_alloc returned NULL")
	}

	if cfg.Vendor != 0 {
		procVigemTargetSetVid.Call(target, uintptr(cfg.Vendor))
	}
	if cfg.Product != 0 {
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

	if ret, _, _ := procVigemTargetX360RegisterNotification.Call(
		client,
		target,
		getNotifyCallback(),
		0,
	); uint32(ret) != vigemErrorNone {
		_ = p.Close()
		return nil, vigemErr("vigem_target_x360_register_notification", ret)
	}

	return p, nil
}

func (p *winPad) Name() string { return p.name }

// Run reads HID input reports from the physical DualSense controller, maps
// them to XUSB_REPORT, and updates the ViGEm virtual Xbox 360 pad until ctx is
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
		p.log("vpad: reading %s, forwarding to ViGEm Xbox 360 pad (%s)", phys.Info(), p.name)
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
			return fmt.Errorf("vpad: read %s: %w", phys.Info().Path, err)
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
		rep := encodeXUSBReport(st, p.cfg.Deadzone)
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
		return nil, fmt.Errorf("vpad: %w", err)
	}
	return device.Open(info)
}

func (p *winPad) updateReport(rep [12]byte) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	var ret uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		// On 64-bit Windows (Microsoft x64 ABI), structs of size 12 bytes are
		// passed by pointer in R8.
		ret, _, _ = procVigemTargetX360Update.Call(
			p.client,
			p.target,
			uintptr(unsafe.Pointer(&rep[0])),
		)
	} else {
		w0 := uintptr(binary.LittleEndian.Uint32(rep[0:4]))
		w1 := uintptr(binary.LittleEndian.Uint32(rep[4:8]))
		w2 := uintptr(binary.LittleEndian.Uint32(rep[8:12]))
		ret, _, _ = procVigemTargetX360Update.Call(p.client, p.target, w0, w1, w2)
	}
	p.mu.Unlock()

	if uint32(ret) != vigemErrorNone {
		return vigemErr("vigem_target_x360_update", ret)
	}
	return nil
}

// Close unregisters notifications, unplugs the virtual Xbox 360 target, and
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
		_, _, _ = procVigemTargetX360Unregister.Call(p.target)
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
