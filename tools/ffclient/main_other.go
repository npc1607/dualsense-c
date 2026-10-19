//go:build !linux

// Command ffclient needs Linux evdev force feedback; on other platforms it is a
// no-op stub so the repository still builds everywhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ffclient: only supported on Linux")
	os.Exit(1)
}
