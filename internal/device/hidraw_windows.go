//go:build windows

package device

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows support uses SetupAPI to find the HID device interface and then a
// plain HID handle for I/O (WriteFile / ReadFile, where the first byte of the
// buffer is the report ID).
//
// STATUS: compiles and mirrors the Linux behaviour, but has not been exercised
// on Windows hardware yet. The Bluetooth report format in particular may need
// adjustment. Treat this as the integration point for Windows, not a
// guaranteed-working implementation.

var (
	modhid      = windows.NewLazySystemDLL("hid.dll")
	modsetupapi = windows.NewLazySystemDLL("setupapi.dll")

	procHidDGetHidGuid                    = modhid.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes                 = modhid.NewProc("HidD_GetAttributes")
	procSetupDiGetClassDevsW              = modsetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces       = modsetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW  = modsetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiGetDeviceRegistryPropertyW = modsetupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList      = modsetupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	digcfPresent         = 0x02
	digcfDeviceInterface = 0x10
	spdrpEnumeratorName  = 0x16
	noMoreItems          = syscall.Errno(259)
)

type spDeviceInterfaceData struct {
	cbSize             uint32
	InterfaceClassGuid windows.GUID
	Flags              uint32
	Reserved           uintptr
}

type spDevInfoData struct {
	cbSize    uint32
	ClassGUID windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

type hidAttributes struct {
	Size          uint32
	VendorID      uint16
	ProductID     uint16
	VersionNumber uint16
}

// FindAll enumerates present HID interfaces and keeps the DualSense family.
func FindAll() []Info {
	var guid windows.GUID
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&guid)))

	devs, _, _ := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&guid)), 0, 0,
		digcfPresent|digcfDeviceInterface,
	)
	if devs == 0 || devs == uintptr(windows.InvalidHandle) {
		return nil
	}
	defer procSetupDiDestroyDeviceInfoList.Call(devs)

	var out []Info
	for i := 0; ; i++ {
		var did spDeviceInterfaceData
		did.cbSize = uint32(unsafe.Sizeof(did))
		ret, _, err := procSetupDiEnumDeviceInterfaces.Call(
			devs, 0, uintptr(unsafe.Pointer(&guid)), uintptr(i),
			uintptr(unsafe.Pointer(&did)),
		)
		if ret == 0 {
			if errno, isErrno := err.(syscall.Errno); isErrno && errno == noMoreItems {
				break
			}
			break
		}

		path, devInfo, good := interfaceDetail(devs, &did)
		if !good {
			continue
		}
		vid, pid, known := attributes(path)
		if !known || !IsDualSense(vid, pid) {
			continue
		}
		tr := USB
		if enumerator(devs, &devInfo) == "BTHENUM" {
			tr = BT
		}
		out = append(out, NewInfo(path, vid, pid, tr))
	}
	return out
}

// Find returns the best candidate.
func Find() (Info, error) {
	all := FindAll()
	if len(all) == 0 {
		return Info{}, fmt.Errorf("device: no DualSense found (plug it in over USB; check `ds5ctl list`)")
	}
	return all[0], nil
}

func interfaceDetail(set uintptr, did *spDeviceInterfaceData) (string, spDevInfoData, bool) {
	var required uint32
	procSetupDiGetDeviceInterfaceDetailW.Call(
		set, uintptr(unsafe.Pointer(did)), 0, 0,
		uintptr(unsafe.Pointer(&required)), 0,
	)
	if required == 0 {
		return "", spDevInfoData{}, false
	}
	buf := make([]byte, required)
	// cbSize differs by architecture: the struct is packed (DWORD + WCHAR[1]).
	cbSize := uint32(8)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		cbSize = 6
	}
	*(*uint32)(unsafe.Pointer(&buf[0])) = cbSize

	var devInfo spDevInfoData
	devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))
	ret, _, _ := procSetupDiGetDeviceInterfaceDetailW.Call(
		set, uintptr(unsafe.Pointer(did)), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(required), 0, uintptr(unsafe.Pointer(&devInfo)),
	)
	if ret == 0 {
		return "", spDevInfoData{}, false
	}
	// DevicePath starts right after the DWORD cbSize.
	p := (*uint16)(unsafe.Pointer(&buf[4]))
	return windows.UTF16PtrToString(p), devInfo, true
}

func attributes(path string) (uint16, uint16, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, 0, false
	}
	defer windows.CloseHandle(h)

	var a hidAttributes
	a.Size = uint32(unsafe.Sizeof(a))
	if ok, _, _ := procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&a))); ok == 0 {
		return 0, 0, false
	}
	return a.VendorID, a.ProductID, true
}

func enumerator(set uintptr, devInfo *spDevInfoData) string {
	buf := make([]uint16, 128)
	var regType, required uint32
	ret, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		set, uintptr(unsafe.Pointer(devInfo)), spdrpEnumeratorName,
		uintptr(unsafe.Pointer(&regType)), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2), uintptr(unsafe.Pointer(&required)),
	)
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

type winDevice struct {
	info Info
	h    windows.Handle
}

// Open opens the HID interface for read/write.
func Open(info Info) (Device, error) {
	p, err := windows.UTF16PtrFromString(info.Path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("device: open %s: %w", info.Path, err)
	}
	return &winDevice{info: info, h: h}, nil
}

// OpenPath opens a device interface path verbatim.
func OpenPath(path string) (Device, error) {
	info := Info{Path: path, Transport: USB, Name: path}
	if vid, pid, ok := attributes(path); ok {
		info.Vendor, info.Product = vid, pid
		info.Name = Describe(vid, pid)
	}
	return Open(info)
}

func (d *winDevice) Info() Info { return d.info }

func (d *winDevice) Write(report []byte) error {
	var n uint32
	return windows.WriteFile(d.h, report, &n, nil)
}

func (d *winDevice) Read(buf []byte) (int, error) {
	var n uint32
	if err := windows.ReadFile(d.h, buf, &n, nil); err != nil {
		return 0, err
	}
	return int(n), nil
}

func (d *winDevice) Readable() bool { return true }

func (d *winDevice) Close() error { return windows.CloseHandle(d.h) }
