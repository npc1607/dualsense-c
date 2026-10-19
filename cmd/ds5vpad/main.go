// Command ds5vpad presents a virtual Xbox pad to the game and republishes the
// game's force feedback, so ds5d can turn it into DualSense haptics.
//
// It exists because a game only sends rumble to the pad it is talking to: if
// Steam Input owns the physical controller, an external tool can never see it.
// With ds5vpad the game talks to our pad, Steam Input can stay off, and no
// other process writes the controller's HID output.
//
// Linux uses /dev/uinput (install udev/71-ds5vpad.rules first). The Windows
// backend is stubbed out; see internal/vpad/vpad_windows.go for the plan.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/source/ffb"
	"github.com/npc1607/dualsense-c/internal/vpad"
)

var version = "dev"

func main() {
	var (
		name    = flag.String("name", "ds5vpad virtual pad", "virtual device name the game will see")
		grab    = flag.String("grab", "", "physical evdev node to grab (default: auto-detect DualSense)")
		publish = flag.String("publish", ffb.DefaultListen, "UDP address to publish rumble to")
		hz      = flag.Int("hz", 60, "rumble publish rate")
		quiet   = flag.Bool("quiet", false, "only log errors")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("ds5vpad %s\n", version)
		return
	}

	logger := log.New(os.Stderr, "ds5vpad: ", log.LstdFlags)
	if *quiet {
		logger.SetOutput(os.Stderr)
	}

	addr, err := net.ResolveUDPAddr("udp4", *publish)
	if err != nil {
		logger.Fatalf("bad -publish: %v", err)
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		logger.Fatalf("dial %s: %v", *publish, err)
	}
	defer conn.Close()

	// The backend reports rumble on arbitrary goroutines; send from one place so
	// the datagrams stay in order and the socket is not shared.
	reports := make(chan vpad.Rumble, 32)
	var seq uint32
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		tick := time.NewTicker(time.Second / time.Duration(*hz))
		defer tick.Stop()
		var last vpad.Rumble
		for {
			select {
			case <-ctx.Done():
				return
			case r := <-reports:
				last = r
			case <-tick.C:
				// Expire a state the game stopped refreshing so the motors do
				// not stay stuck on after a pause or a level change.
				if !last.At.IsZero() && time.Since(last.At) > 500*time.Millisecond {
					last = vpad.Rumble{At: last.At}
				}
				seq++
				if _, err := conn.Write(ffb.Encode(last.Left, last.Right, seq)); err != nil {
					logger.Printf("publish: %v", err)
				}
			}
		}
	}()

	cfg := vpad.DefaultConfig()
	cfg.Name = *name
	cfg.GrabPath = *grab
	cfg.Logger = logger

	sink := func(r vpad.Rumble) {
		select {
		case reports <- r:
		default: // drop rather than block the FF control channel
		}
	}

	pad, err := vpad.Open(cfg, sink)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	defer pad.Close()
	logger.Printf("virtual pad: %s", pad.Name())

	if !*quiet {
		logger.Printf("forwarding input from the physical controller; publishing rumble to %s at %d Hz", *publish, *hz)
	}
	if err := pad.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Fatalf("%v", err)
	}
	atomic.AddUint32(&seq, 0) // keep the counter alive for the deferred send path
	_ = conn.Close()
}
