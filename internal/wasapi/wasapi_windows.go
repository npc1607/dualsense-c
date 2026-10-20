//go:build windows

package wasapi

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32 = windows.NewLazySystemDLL("ole32.dll")

	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = modOle32.NewProc("CoTaskMemFree")
	procPropVariantClear = modOle32.NewProc("PropVariantClear")
)

const (
	coinitMultithreaded = 0x0
	clsctxAll           = 0x17
	stgmRead            = 0x00000000

	eRender  = 0
	eConsole = 0

	deviceStateActive = 0x00000001

	audclntShareModeShared              = 0
	audclntStreamFlagsLoopback          = 0x00020000
	audclntStreamFlagsAutoConvertPCM    = 0x80000000
	audclntStreamFlagsSRCDefaultQuality = 0x08000000
	audclntBufferFlagsSilent            = 0x00000002

	vtLPWSTR = 31

	rpcEChangedMode = 0x80010106
)

var (
	clsidMMDeviceEnumerator = windows.GUID{
		Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C,
		Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E},
	}
	iidIMMDeviceEnumerator = windows.GUID{
		Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35,
		Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6},
	}
	iidIAudioClient = windows.GUID{
		Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4C32,
		Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2},
	}
	iidIAudioCaptureClient = windows.GUID{
		Data1: 0xC8ADBD64, Data2: 0xE71E, Data3: 0x48A0,
		Data4: [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17},
	}
	iidIAudioRenderClient = windows.GUID{
		Data1: 0xF294ACFC, Data2: 0x3146, Data3: 0x4483,
		Data4: [8]byte{0xA7, 0xBF, 0xAD, 0xDC, 0xA7, 0xC2, 0x60, 0xE2},
	}
	pkeyDeviceFriendlyName = propertyKey{
		fmtid: windows.GUID{
			Data1: 0xA45C254E, Data2: 0xDF1C, Data3: 0x4EFD,
			Data4: [8]byte{0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0},
		},
		pid: 14,
	}
)

type comObject struct {
	vtbl *[32]uintptr
}

func (c *comObject) ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

func (c *comObject) method(index int) uintptr {
	return c.vtbl[index]
}

func comRelease(obj *comObject) {
	if obj == nil {
		return
	}
	_, _, _ = syscall.SyscallN(obj.method(2), obj.ptr())
}

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

type propVariant struct {
	vt         uint16
	wReserved1 uint16
	wReserved2 uint16
	wReserved3 uint16
	pwszVal    *uint16
	pad        uintptr
}

func coInit() error {
	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	if hr != 0 && hr != 1 && uint32(hr) != rpcEChangedMode {
		return fmt.Errorf("wasapi: CoInitializeEx failed: HRESULT 0x%08x", uint32(hr))
	}
	return nil
}

func createEnumerator() (*comObject, error) {
	if err := coInit(); err != nil {
		return nil, err
	}
	var enum *comObject
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enum)),
	)
	if hr != 0 || enum == nil {
		return nil, fmt.Errorf("wasapi: CoCreateInstance(IMMDeviceEnumerator) failed: HRESULT 0x%08x", uint32(hr))
	}
	return enum, nil
}

func deviceFriendlyName(dev *comObject) string {
	// IMMDevice::OpenPropertyStore is vtable index 4.
	var store *comObject
	hr, _, _ := syscall.SyscallN(dev.method(4), dev.ptr(), stgmRead, uintptr(unsafe.Pointer(&store)))
	if hr != 0 || store == nil {
		return ""
	}
	defer comRelease(store)

	// IPropertyStore::GetValue is vtable index 5.
	var pv propVariant
	hr, _, _ = syscall.SyscallN(
		store.method(5),
		store.ptr(),
		uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)),
		uintptr(unsafe.Pointer(&pv)),
	)
	if hr != 0 {
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))

	if pv.vt == vtLPWSTR && pv.pwszVal != nil {
		return windows.UTF16PtrToString(pv.pwszVal)
	}
	return ""
}

func deviceID(dev *comObject) string {
	// IMMDevice::GetId is vtable index 5.
	var idPtr *uint16
	hr, _, _ := syscall.SyscallN(dev.method(5), dev.ptr(), uintptr(unsafe.Pointer(&idPtr)))
	if hr != 0 || idPtr == nil {
		return ""
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(idPtr)))
	return windows.UTF16PtrToString(idPtr)
}

// DefaultRenderName returns the friendly name of the default eRender endpoint.
func DefaultRenderName() (string, error) {
	enum, err := createEnumerator()
	if err != nil {
		return "", err
	}
	defer comRelease(enum)

	// IMMDeviceEnumerator::GetDefaultAudioEndpoint is vtable index 4.
	var dev *comObject
	hr, _, _ := syscall.SyscallN(enum.method(4), enum.ptr(), eRender, eConsole, uintptr(unsafe.Pointer(&dev)))
	if hr != 0 || dev == nil {
		return "", fmt.Errorf("wasapi: GetDefaultAudioEndpoint failed: HRESULT 0x%08x", uint32(hr))
	}
	defer comRelease(dev)

	if name := deviceFriendlyName(dev); name != "" {
		return name, nil
	}
	if id := deviceID(dev); id != "" {
		return id, nil
	}
	return "default", nil
}

// FindRenderDevice searches active eRender endpoints for one whose friendly
// name or ID matches any of the given keywords (case-insensitive).
func FindRenderDevice(keywords ...string) (string, error) {
	enum, err := createEnumerator()
	if err != nil {
		return "", err
	}
	defer comRelease(enum)

	// IMMDeviceEnumerator::EnumAudioEndpoints is vtable index 3.
	var coll *comObject
	hr, _, _ := syscall.SyscallN(enum.method(3), enum.ptr(), eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll)))
	if hr != 0 || coll == nil {
		return "", fmt.Errorf("wasapi: EnumAudioEndpoints failed: HRESULT 0x%08x", uint32(hr))
	}
	defer comRelease(coll)

	// IMMDeviceCollection::GetCount is vtable index 3.
	var count uint32
	hr, _, _ = syscall.SyscallN(coll.method(3), coll.ptr(), uintptr(unsafe.Pointer(&count)))
	if hr != 0 {
		return "", fmt.Errorf("wasapi: IMMDeviceCollection::GetCount failed: HRESULT 0x%08x", uint32(hr))
	}

	for i := uint32(0); i < count; i++ {
		// IMMDeviceCollection::Item is vtable index 4.
		var dev *comObject
		hr, _, _ := syscall.SyscallN(coll.method(4), coll.ptr(), uintptr(i), uintptr(unsafe.Pointer(&dev)))
		if hr != 0 || dev == nil {
			continue
		}
		name := deviceFriendlyName(dev)
		id := deviceID(dev)
		comRelease(dev)

		lowName := strings.ToLower(name)
		lowID := strings.ToLower(id)
		for _, kw := range keywords {
			k := strings.ToLower(kw)
			if k != "" && (strings.Contains(lowName, k) || strings.Contains(lowID, k)) {
				if name != "" {
					return name, nil
				}
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("wasapi: no matching audio render endpoint found for %v", keywords)
}

func resolveRenderEndpoint(enum *comObject, target string) (*comObject, string, error) {
	trimmed := strings.TrimSpace(target)
	low := strings.ToLower(trimmed)
	if trimmed == "" || low == "auto" || low == "default" || strings.HasSuffix(low, ".monitor") {
		var dev *comObject
		hr, _, _ := syscall.SyscallN(enum.method(4), enum.ptr(), eRender, eConsole, uintptr(unsafe.Pointer(&dev)))
		if hr != 0 || dev == nil {
			return nil, "", fmt.Errorf("wasapi: GetDefaultAudioEndpoint failed: HRESULT 0x%08x", uint32(hr))
		}
		name := deviceFriendlyName(dev)
		if name == "" {
			name = "default"
		}
		return dev, name, nil
	}

	var coll *comObject
	hr, _, _ := syscall.SyscallN(enum.method(3), enum.ptr(), eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll)))
	if hr != 0 || coll == nil {
		return nil, "", fmt.Errorf("wasapi: EnumAudioEndpoints failed: HRESULT 0x%08x", uint32(hr))
	}
	defer comRelease(coll)

	var count uint32
	hr, _, _ = syscall.SyscallN(coll.method(3), coll.ptr(), uintptr(unsafe.Pointer(&count)))
	if hr != 0 {
		return nil, "", fmt.Errorf("wasapi: GetCount failed: HRESULT 0x%08x", uint32(hr))
	}

	for i := uint32(0); i < count; i++ {
		var dev *comObject
		hr, _, _ := syscall.SyscallN(coll.method(4), coll.ptr(), uintptr(i), uintptr(unsafe.Pointer(&dev)))
		if hr != 0 || dev == nil {
			continue
		}
		name := deviceFriendlyName(dev)
		id := deviceID(dev)
		if strings.Contains(strings.ToLower(name), low) || strings.Contains(strings.ToLower(id), low) {
			if name == "" {
				name = id
			}
			return dev, name, nil
		}
		comRelease(dev)
	}
	return nil, "", fmt.Errorf("wasapi: render device %q not found", target)
}

// LoopbackStream captures system audio from a WASAPI render endpoint in
// shared-mode loopback as interleaved float32le PCM.
type LoopbackStream struct {
	enum       *comObject
	dev        *comObject
	client     *comObject
	capture    *comObject
	name       string
	frameBytes int
	pending    []byte
	deadlineNS atomic.Int64

	mu     sync.Mutex
	closed bool
}

// OpenLoopback opens a WASAPI shared-mode loopback capture stream on the
// target render endpoint ("", "auto", "default", or a substring of the device
// friendly name) and converts audio to interleaved float32le at rate/channels.
func OpenLoopback(target string, rate, channels int) (*LoopbackStream, error) {
	if rate <= 0 || channels <= 0 {
		return nil, errors.New("wasapi: rate and channels must be positive")
	}
	enum, err := createEnumerator()
	if err != nil {
		return nil, err
	}
	dev, name, err := resolveRenderEndpoint(enum, target)
	if err != nil {
		comRelease(enum)
		return nil, err
	}

	// IMMDevice::Activate is vtable index 3.
	var client *comObject
	hr, _, _ := syscall.SyscallN(
		dev.method(3),
		dev.ptr(),
		uintptr(unsafe.Pointer(&iidIAudioClient)),
		clsctxAll,
		0,
		uintptr(unsafe.Pointer(&client)),
	)
	if hr != 0 || client == nil {
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: Activate(IAudioClient) failed: HRESULT 0x%08x", uint32(hr))
	}

	wf := BuildFloat32Format(rate, channels)
	const hnsBufferDuration = 200000 // 20 ms in 100-ns units
	flags := uintptr(audclntStreamFlagsLoopback | audclntStreamFlagsAutoConvertPCM | audclntStreamFlagsSRCDefaultQuality)

	// IAudioClient::Initialize is vtable index 3.
	hr, _, _ = syscall.SyscallN(
		client.method(3),
		client.ptr(),
		audclntShareModeShared,
		flags,
		hnsBufferDuration,
		0,
		uintptr(unsafe.Pointer(&wf[0])),
		0,
	)
	if hr != 0 {
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: IAudioClient::Initialize(loopback %d Hz %dch) failed: HRESULT 0x%08x", rate, channels, uint32(hr))
	}

	// IAudioClient::GetService is vtable index 14.
	var capture *comObject
	hr, _, _ = syscall.SyscallN(
		client.method(14),
		client.ptr(),
		uintptr(unsafe.Pointer(&iidIAudioCaptureClient)),
		uintptr(unsafe.Pointer(&capture)),
	)
	if hr != 0 || capture == nil {
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: GetService(IAudioCaptureClient) failed: HRESULT 0x%08x", uint32(hr))
	}

	// IAudioClient::Start is vtable index 10.
	hr, _, _ = syscall.SyscallN(client.method(10), client.ptr())
	if hr != 0 {
		comRelease(capture)
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: IAudioClient::Start failed: HRESULT 0x%08x", uint32(hr))
	}

	return &LoopbackStream{
		enum:       enum,
		dev:        dev,
		client:     client,
		capture:    capture,
		name:       name,
		frameBytes: channels * 4,
	}, nil
}

// Name returns the resolved endpoint friendly name.
func (s *LoopbackStream) Name() string { return s.name }

// SetReadDeadline sets the deadline for future Read calls. A zero time value
// disables the deadline.
func (s *LoopbackStream) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		s.deadlineNS.Store(0)
	} else {
		s.deadlineNS.Store(t.UnixNano())
	}
	return nil
}

// Read reads interleaved float32le PCM bytes from the loopback stream.
func (s *LoopbackStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.pending) > 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}

	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return 0, io.EOF
		}

		// IAudioCaptureClient::GetNextPacketSize is vtable index 5.
		var packetFrames uint32
		hr, _, _ := syscall.SyscallN(
			s.capture.method(5),
			s.capture.ptr(),
			uintptr(unsafe.Pointer(&packetFrames)),
		)
		if hr != 0 {
			s.mu.Unlock()
			return 0, fmt.Errorf("wasapi: GetNextPacketSize failed: HRESULT 0x%08x", uint32(hr))
		}

		if packetFrames == 0 {
			s.mu.Unlock()
			if dl := s.deadlineNS.Load(); dl > 0 && time.Now().UnixNano() >= dl {
				return 0, os.ErrDeadlineExceeded
			}
			time.Sleep(2 * time.Millisecond)
			continue
		}

		// IAudioCaptureClient::GetBuffer is vtable index 3.
		var (
			dataPtr   *byte
			numFrames uint32
			flags     uint32
			devPos    uint64
			qpcPos    uint64
		)
		hr, _, _ = syscall.SyscallN(
			s.capture.method(3),
			s.capture.ptr(),
			uintptr(unsafe.Pointer(&dataPtr)),
			uintptr(unsafe.Pointer(&numFrames)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&devPos)),
			uintptr(unsafe.Pointer(&qpcPos)),
		)
		if hr != 0 {
			s.mu.Unlock()
			return 0, fmt.Errorf("wasapi: GetBuffer failed: HRESULT 0x%08x", uint32(hr))
		}

		totalBytes := int(numFrames) * s.frameBytes
		if totalBytes > 0 {
			if flags&audclntBufferFlagsSilent != 0 || dataPtr == nil {
				s.pending = append(s.pending, make([]byte, totalBytes)...)
			} else {
				src := unsafe.Slice(dataPtr, totalBytes)
				s.pending = append(s.pending, src...)
			}
		}

		// IAudioCaptureClient::ReleaseBuffer is vtable index 4.
		_, _, _ = syscall.SyscallN(s.capture.method(4), s.capture.ptr(), uintptr(numFrames))
		s.mu.Unlock()

		if len(s.pending) > 0 {
			n := copy(p, s.pending)
			s.pending = s.pending[n:]
			return n, nil
		}
	}
}

// Close stops the WASAPI capture stream and releases COM interfaces.
func (s *LoopbackStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.client != nil {
		// IAudioClient::Stop is vtable index 11.
		_, _, _ = syscall.SyscallN(s.client.method(11), s.client.ptr())
	}
	comRelease(s.capture)
	comRelease(s.client)
	comRelease(s.dev)
	comRelease(s.enum)
	s.capture, s.client, s.dev, s.enum = nil, nil, nil, nil
	return nil
}

// RenderStream writes 4-channel s16le quadraphonic PCM to a WASAPI endpoint
// (channels 1/2 front speaker/headset, channels 3/4 DualSense voice coils).
type RenderStream struct {
	enum         *comObject
	dev          *comObject
	client       *comObject
	render       *comObject
	name         string
	bufferFrames uint32
	frameBytes   int

	mu     sync.Mutex
	closed bool
}

// OpenRender opens a 4-channel 16-bit PCM shared-mode WASAPI render stream on
// the target endpoint.
func OpenRender(target string, rate int) (*RenderStream, error) {
	if rate <= 0 {
		return nil, errors.New("wasapi: rate must be positive")
	}
	enum, err := createEnumerator()
	if err != nil {
		return nil, err
	}
	dev, name, err := resolveRenderEndpoint(enum, target)
	if err != nil {
		comRelease(enum)
		return nil, err
	}

	var client *comObject
	hr, _, _ := syscall.SyscallN(
		dev.method(3),
		dev.ptr(),
		uintptr(unsafe.Pointer(&iidIAudioClient)),
		clsctxAll,
		0,
		uintptr(unsafe.Pointer(&client)),
	)
	if hr != 0 || client == nil {
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: Activate(IAudioClient) failed: HRESULT 0x%08x", uint32(hr))
	}

	wf := BuildQuadS16Format(rate)
	const hnsBufferDuration = 200000 // 20 ms
	flags := uintptr(audclntStreamFlagsAutoConvertPCM | audclntStreamFlagsSRCDefaultQuality)

	hr, _, _ = syscall.SyscallN(
		client.method(3),
		client.ptr(),
		audclntShareModeShared,
		flags,
		hnsBufferDuration,
		0,
		uintptr(unsafe.Pointer(&wf[0])),
		0,
	)
	if hr != 0 {
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: IAudioClient::Initialize(render 4ch %d Hz) failed: HRESULT 0x%08x", rate, uint32(hr))
	}

	// IAudioClient::GetBufferSize is vtable index 4.
	var bufferFrames uint32
	hr, _, _ = syscall.SyscallN(
		client.method(4),
		client.ptr(),
		uintptr(unsafe.Pointer(&bufferFrames)),
	)
	if hr != 0 || bufferFrames == 0 {
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: GetBufferSize failed: HRESULT 0x%08x", uint32(hr))
	}

	// IAudioClient::GetService is vtable index 14.
	var render *comObject
	hr, _, _ = syscall.SyscallN(
		client.method(14),
		client.ptr(),
		uintptr(unsafe.Pointer(&iidIAudioRenderClient)),
		uintptr(unsafe.Pointer(&render)),
	)
	if hr != 0 || render == nil {
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: GetService(IAudioRenderClient) failed: HRESULT 0x%08x", uint32(hr))
	}

	// IAudioClient::Start is vtable index 10.
	hr, _, _ = syscall.SyscallN(client.method(10), client.ptr())
	if hr != 0 {
		comRelease(render)
		comRelease(client)
		comRelease(dev)
		comRelease(enum)
		return nil, fmt.Errorf("wasapi: IAudioClient::Start failed: HRESULT 0x%08x", uint32(hr))
	}

	return &RenderStream{
		enum:         enum,
		dev:          dev,
		client:       client,
		render:       render,
		name:         name,
		bufferFrames: bufferFrames,
		frameBytes:   8, // 4 channels * 2 bytes (s16le)
	}, nil
}

// Name returns the resolved endpoint friendly name.
func (s *RenderStream) Name() string { return s.name }

// Write writes 4-channel s16le PCM frames to the WASAPI render buffer.
func (s *RenderStream) Write(p []byte) (int, error) {
	totalFrames := len(p) / s.frameBytes
	if totalFrames == 0 {
		return len(p), nil
	}

	written := 0
	for written < totalFrames {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return written * s.frameBytes, io.ErrClosedPipe
		}

		// IAudioClient::GetCurrentPadding is vtable index 6.
		var padding uint32
		hr, _, _ := syscall.SyscallN(
			s.client.method(6),
			s.client.ptr(),
			uintptr(unsafe.Pointer(&padding)),
		)
		if hr != 0 {
			s.mu.Unlock()
			return written * s.frameBytes, fmt.Errorf("wasapi: GetCurrentPadding failed: HRESULT 0x%08x", uint32(hr))
		}

		avail := int(s.bufferFrames) - int(padding)
		if avail <= 0 {
			s.mu.Unlock()
			time.Sleep(time.Millisecond)
			continue
		}

		chunk := totalFrames - written
		if chunk > avail {
			chunk = avail
		}

		// IAudioRenderClient::GetBuffer is vtable index 3.
		var dataPtr *byte
		hr, _, _ = syscall.SyscallN(
			s.render.method(3),
			s.render.ptr(),
			uintptr(chunk),
			uintptr(unsafe.Pointer(&dataPtr)),
		)
		if hr != 0 || dataPtr == nil {
			s.mu.Unlock()
			return written * s.frameBytes, fmt.Errorf("wasapi: IAudioRenderClient::GetBuffer failed: HRESULT 0x%08x", uint32(hr))
		}

		byteCount := chunk * s.frameBytes
		dst := unsafe.Slice(dataPtr, byteCount)
		srcOff := written * s.frameBytes
		copy(dst, p[srcOff:srcOff+byteCount])

		// IAudioRenderClient::ReleaseBuffer is vtable index 4.
		hr, _, _ = syscall.SyscallN(
			s.render.method(4),
			s.render.ptr(),
			uintptr(chunk),
			0,
		)
		s.mu.Unlock()
		if hr != 0 {
			return written * s.frameBytes, fmt.Errorf("wasapi: IAudioRenderClient::ReleaseBuffer failed: HRESULT 0x%08x", uint32(hr))
		}
		written += chunk
	}
	return len(p), nil
}

// Close stops the WASAPI render stream and releases COM interfaces.
func (s *RenderStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.client != nil {
		_, _, _ = syscall.SyscallN(s.client.method(11), s.client.ptr())
	}
	comRelease(s.render)
	comRelease(s.client)
	comRelease(s.dev)
	comRelease(s.enum)
	s.render, s.client, s.dev, s.enum = nil, nil, nil, nil
	return nil
}
