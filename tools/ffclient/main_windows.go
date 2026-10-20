//go:build windows

// Command ffclient sends XInput rumble commands to an XInput controller slot
// (such as the virtual Xbox 360 pad created by ds5vpad.exe via ViGEmBus).
//
//	.\bin\ds5vpad.exe
//	.\bin\ds5d.exe -profile ffb-recoil -trace
//	.\bin\ffclient.exe -dev 0
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type xinputVibration struct {
	LeftMotorSpeed  uint16
	RightMotorSpeed uint16
}

type xinputState struct {
	PacketNumber uint32
	Gamepad      [12]byte
}

func loadXInput() (*windows.LazyProc, *windows.LazyProc, error) {
	for _, dllName := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		dll := windows.NewLazySystemDLL(dllName)
		if err := dll.Load(); err == nil {
			setProc := dll.NewProc("XInputSetState")
			getProc := dll.NewProc("XInputGetState")
			if setProc.Find() == nil && getProc.Find() == nil {
				return setProc, getProc, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no XInput DLL found (xinput1_4.dll / xinput1_3.dll / xinput9_1_0.dll)")
}

func main() {
	dev := flag.String("dev", "", "XInput user index 0..3 (default: first connected controller or 0)")
	strong := flag.Int("strong", 40000, "strong (low frequency) magnitude 0..65535")
	weak := flag.Int("weak", 20000, "weak (high frequency) magnitude 0..65535")
	ms := flag.Int("ms", 1200, "duration in milliseconds")
	flag.Parse()

	setProc, getProc, err := loadXInput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ffclient: %v\n", err)
		os.Exit(1)
	}

	userIdx := uint32(0)
	if *dev != "" {
		n, err := strconv.Atoi(*dev)
		if err != nil || n < 0 || n > 3 {
			fmt.Fprintf(os.Stderr, "ffclient: -dev must be an XInput controller index 0..3 (got %q)\n", *dev)
			os.Exit(2)
		}
		userIdx = uint32(n)
	} else {
		for i := uint32(0); i < 4; i++ {
			var st xinputState
			if ret, _, _ := getProc.Call(uintptr(i), uintptr(unsafe.Pointer(&st))); ret == 0 {
				userIdx = i
				break
			}
		}
	}

	play := xinputVibration{
		LeftMotorSpeed:  uint16(*strong),
		RightMotorSpeed: uint16(*weak),
	}
	stop := xinputVibration{}

	fmt.Printf("sending XInput rumble to controller index=%d strong=%d weak=%d\n", userIdx, *strong, *weak)
	for i := 0; i < 3; i++ {
		if ret, _, _ := setProc.Call(uintptr(userIdx), uintptr(unsafe.Pointer(&play))); ret != 0 {
			fmt.Fprintf(os.Stderr, "XInputSetState(index=%d): error %d (is ds5vpad.exe running?)\n", userIdx, ret)
			os.Exit(1)
		}
		fmt.Println("play")
		time.Sleep(time.Duration(*ms) * time.Millisecond)

		_, _, _ = setProc.Call(uintptr(userIdx), uintptr(unsafe.Pointer(&stop)))
		fmt.Println("stop")
		time.Sleep(300 * time.Millisecond)
	}
}
