//go:build linux

// Command ffclient plays a rumble effect on an evdev node, which is what a
// game does when it wants the pad to shake.
//
// It exists to verify the force-feedback chain without launching a game:
//
//	./bin/ds5vpad &                 # creates + grabs, publishes to 9998
//	./bin/ds5d -profile ffb-recoil -trace &   # consumes 9998
//	sudo ./bin/ffclient -dev /dev/input/eventN   # shake, watch the trace
//
// (sudo is only needed if the node is not readable by your user; uinput pads
// usually get a uaccess ACL automatically.)
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// EVIOCSFF = _IOW('E', 0x80, struct ff_effect); ff_effect is 48 bytes.
const evIOCSFF = 0x40304580

const (
	ffRumble = 0x50
	evFF     = 0x15
)

func main() {
	dev := flag.String("dev", "", "evdev node of the virtual pad (required)")
	strong := flag.Int("strong", 40000, "strong (low frequency) magnitude 0..65535")
	weak := flag.Int("weak", 20000, "weak (high frequency) magnitude 0..65535")
	ms := flag.Int("ms", 1200, "duration in milliseconds")
	flag.Parse()

	if *dev == "" {
		fmt.Fprintln(os.Stderr, "usage: ffclient -dev /dev/input/eventN")
		os.Exit(2)
	}
	fd, err := unix.Open(*dev, unix.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dev, err)
		os.Exit(1)
	}
	defer unix.Close(fd)

	eff := make([]byte, 48)
	binary.LittleEndian.PutUint16(eff[0:], ffRumble)
	binary.LittleEndian.PutUint16(eff[2:], ^uint16(0)) // id := -1 (let the kernel assign)
	binary.LittleEndian.PutUint16(eff[10:], uint16(*ms))
	binary.LittleEndian.PutUint16(eff[12:], uint16(*strong))
	binary.LittleEndian.PutUint16(eff[14:], uint16(*weak))

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), evIOCSFF,
		uintptr(unsafe.Pointer(&eff[0]))); errno != 0 {
		fmt.Fprintf(os.Stderr, "EVIOCSFF: %v\n", errno)
		os.Exit(1)
	}
	id := binary.LittleEndian.Uint16(eff[2:])
	fmt.Printf("uploaded rumble effect id=%d strong=%d weak=%d\n", id, *strong, *weak)

	for i := 0; i < 3; i++ {
		writeEvent(fd, evFF, id, 1) // play
		fmt.Println("play")
		time.Sleep(time.Duration(*ms) * time.Millisecond)
		writeEvent(fd, evFF, id, 0) // stop
		fmt.Println("stop")
		time.Sleep(300 * time.Millisecond)
	}
	// EVIOCRMFF = _IOW('E', 0x81, int)
	unix.Syscall(unix.SYS_IOCTL, uintptr(fd), 0x40044581, uintptr(id))
}

func writeEvent(fd int, typ, code uint16, val int32) {
	var ev [24]byte
	binary.LittleEndian.PutUint16(ev[16:], typ)
	binary.LittleEndian.PutUint16(ev[18:], code)
	binary.LittleEndian.PutUint32(ev[20:], uint32(val))
	unix.Write(fd, ev[:])
}
