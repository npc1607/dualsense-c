//go:build !linux && !windows

// Command ffclient needs Linux evdev or Windows XInput force feedback; on other
// platforms it is a no-op stub so the repository still builds everywhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ffclient: only supported on Linux and Windows")
	os.Exit(1)
}
