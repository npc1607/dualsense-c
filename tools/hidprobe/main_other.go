//go:build !linux

// Command hidprobe needs Linux hidraw; this stub keeps the tree building.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "hidprobe: only supported on Linux")
	os.Exit(1)
}
