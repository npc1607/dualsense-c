// Package device abstracts the controller transport. Everything above it works
// with hid.Frame values only; Linux talks to /dev/hidrawN, Windows to a HID
// device interface.
package device

import (
	"errors"
	"fmt"
)

// Transport selects the output report format.
type Transport uint8

const (
	USB Transport = iota
	BT
)

func (t Transport) String() string {
	if t == BT {
		return "bluetooth"
	}
	return "usb"
}

// Info describes a discovered controller.
type Info struct {
	Path      string
	Vendor    uint16
	Product   uint16
	Transport Transport
	Name      string
}

// ReportLen is the output report size for this transport.
func (i Info) ReportLen() int {
	if i.Transport == BT {
		return 78
	}
	return 48
}

func (i Info) String() string {
	return fmt.Sprintf("%s [%s] %04x:%04x %s", i.Path, i.Transport, i.Vendor, i.Product, i.Name)
}

// Device is an opened controller.
type Device interface {
	Info() Info
	// Write sends one output report. report must be Info().ReportLen() bytes.
	Write(report []byte) error
	// Read blocks for one input report (used by input-driven sources).
	Read(buf []byte) (int, error)
	// Readable reports whether Read is supported on this platform/transport.
	Readable() bool
	Close() error
}

// ErrUnsupported is returned when this OS cannot talk to the controller.
var ErrUnsupported = errors.New("device: controller access is not implemented on this platform")

var known = []struct {
	vid, pid uint16
	name     string
}{
	{0x054c, 0x0ce6, "DualSense"},
	{0x054c, 0x0df2, "DualSense Edge"},
	{0x054c, 0x0ce6, "DualSense (clone)"},
}

// Describe returns the device name for a VID/PID pair, or "" if unknown.
func Describe(vid, pid uint16) string {
	for _, k := range known {
		if k.vid == vid && k.pid == pid {
			return k.name
		}
	}
	return ""
}

// IsDualSense reports whether the pair belongs to the DualSense family. The
// adaptive-trigger protocol is identical across the family.
func IsDualSense(vid, pid uint16) bool { return Describe(vid, pid) != "" }

// NewInfo fills in the name for a discovered device.
func NewInfo(path string, vid, pid uint16, tr Transport) Info {
	return Info{Path: path, Vendor: vid, Product: pid, Transport: tr, Name: Describe(vid, pid)}
}
