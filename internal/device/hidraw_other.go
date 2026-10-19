//go:build darwin || freebsd || openbsd || netbsd

package device

import "fmt"

// FindAll is unimplemented here: macOS would need an IOHIDManager bridge
// (hidapi / cgo) to reach the DualSense. The rest of the pipeline (profiles,
// rules, `ds5d -dry-run`, `ds5d -trace`) works without a device, which is all
// that is needed for profile development on these platforms.
func FindAll() []Info { return nil }

func Find() (Info, error) {
	return Info{}, fmt.Errorf("%w: use -dry-run to test profiles", ErrUnsupported)
}

func Open(Info) (Device, error) {
	return nil, ErrUnsupported
}

func OpenPath(string) (Device, error) {
	return nil, ErrUnsupported
}
