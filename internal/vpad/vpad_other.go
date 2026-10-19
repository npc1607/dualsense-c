//go:build !linux && !windows

package vpad

import "fmt"

// Open returns ErrUnsupported on platforms without a virtual-pad backend.
func Open(cfg Config, sink RumbleSink) (Pad, error) {
	return nil, fmt.Errorf("%w: no backend for this OS", ErrUnsupported)
}

// DeviceExists is only meaningful where a backend exists.
func DeviceExists(name string) bool { return false }
