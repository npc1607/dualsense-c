//go:build linux

package device

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const sysfsHidraw = "/sys/class/hidraw"

// FindAll lists every DualSense-family hidraw node, sorted with the most
// likely gamepad interface first.
func FindAll() []Info {
	entries, err := os.ReadDir(sysfsHidraw)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "hidraw") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []Info
	for _, n := range names {
		info, ok := probe(n)
		if !ok {
			continue
		}
		// Prefer the main gamepad interface (USB interface 0, ":1.0"), which
		// is the node that accepts the 48-byte trigger report.
		full, err := filepath.EvalSymlinks(filepath.Join(sysfsHidraw, n, "device"))
		if err == nil && strings.Contains(full, ":1.0") {
			out = append([]Info{info}, out...)
			continue
		}
		out = append(out, info)
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

// probe parses /sys/class/hidraw/<node>/device/uevent.
//
//	HID_ID=0003:0000054C:00000CE6   (bus:vendor:product, hex)
//	HID_NAME=Wireless Controller
//
// bus 0003 = USB, 0005 = Bluetooth.
func probe(node string) (Info, bool) {
	raw, err := os.ReadFile(filepath.Join(sysfsHidraw, node, "device", "uevent"))
	if err != nil {
		return Info{}, false
	}
	var info Info
	found := false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "HID_ID="):
			parts := strings.Split(strings.TrimPrefix(line, "HID_ID="), ":")
			if len(parts) != 3 {
				continue
			}
			bus, err1 := strconv.ParseUint(parts[0], 16, 32)
			vid, err2 := strconv.ParseUint(parts[1], 16, 32)
			pid, err3 := strconv.ParseUint(parts[2], 16, 32)
			if err1 != nil || err2 != nil || err3 != nil {
				continue
			}
			info.Vendor = uint16(vid)
			info.Product = uint16(pid)
			if bus == 0x0005 {
				info.Transport = BT
			} else {
				info.Transport = USB
			}
			found = true
		case strings.HasPrefix(line, "HID_NAME="):
			info.Name = strings.TrimSpace(strings.TrimPrefix(line, "HID_NAME="))
		}
	}
	if !found || !IsDualSense(info.Vendor, info.Product) {
		return Info{}, false
	}
	if info.Name == "" {
		info.Name = Describe(info.Vendor, info.Product)
	}
	info.Path = "/dev/" + node
	return info, true
}

type hidraw struct {
	info Info
	f    *os.File
}

// Open opens a hidraw node read/write (the daemon needs write for triggers and
// read for input-driven profiles).
func Open(info Info) (Device, error) {
	f, err := os.OpenFile(info.Path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("device: open %s: %w (install udev/70-dualsense.rules or run as root)", info.Path, err)
	}
	return &hidraw{info: info, f: f}, nil
}

// OpenPath opens an arbitrary hidraw node, bypassing VID/PID detection.
func OpenPath(path string) (Device, error) {
	info := Info{Path: path, Transport: USB, Name: filepath.Base(path)}
	if node := filepath.Base(path); strings.HasPrefix(node, "hidraw") {
		if p, ok := probe(node); ok {
			info = p
		}
	}
	return Open(info)
}

func (d *hidraw) Info() Info { return d.info }

func (d *hidraw) Write(report []byte) error {
	_, err := d.f.Write(report)
	return err
}

func (d *hidraw) Read(buf []byte) (int, error) { return d.f.Read(buf) }

func (d *hidraw) Readable() bool { return true }

func (d *hidraw) Close() error { return d.f.Close() }
