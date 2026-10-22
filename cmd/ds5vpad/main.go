// Command ds5vpad presents a virtual Xbox pad to the game and republishes the
// game's force feedback, so ds5d can turn it into DualSense haptics.
//
// It exists because a game only sends rumble to the pad it is talking to: if
// Steam Input owns the physical controller, an external tool can never see it.
// With ds5vpad the game talks to our pad, Steam Input can stay off, and no
// other process writes the controller's HID output.
//
// Linux uses /dev/uinput (install udev/71-ds5vpad.rules first). The Windows
// backend uses ViGEmBus (XInput via Xbox 360 target, DirectInput 8 via DualShock 4 target).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/vpad"
	_ "github.com/npc1607/dualsense-c/internal/vpad/all"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"
)

var version = "dev"

func main() {
	var (
		proto   = flag.String("proto", string(vpad.ProtocolXInput), "virtual pad protocol: xinput (default, Xbox 360) or dinput8 (Logitech Dual Action DirectInput 8)")
		name    = flag.String("name", "", "virtual device name (default: auto per protocol)")
		grab    = flag.String("grab", "", "physical evdev node to grab (default: auto-detect DualSense)")
		listen  = flag.String("listen", ipc.DefaultEndpoint, "ultra-low latency IPC endpoint for force feedback (e.g. 127.0.0.1:9998 or unix:///tmp/ds5vpad.sock)")
		publish  = flag.String("publish", "", "alias for -listen (deprecated)")
		profPath = flag.String("profile", "", "path to profile .yaml (for aim assist & curves)")
		quiet    = flag.Bool("quiet", false, "only log errors")
		showVer  = flag.Bool("version", false, "print version and exit")
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

	endpoint := *listen
	if *publish != "" {
		endpoint = *publish
	}

	// Start ultra-low latency event-driven IPC server.
	// If no consumers connect, broadcast is a zero-alloc no-op (no socket blasting, no error spam).
	srv, err := ipc.Listen(endpoint, logger)
	if err != nil {
		logger.Fatalf("ipc listen: %v", err)
	}
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var cfg vpad.Config
	switch vpad.Protocol(*proto) {
	case vpad.ProtocolDInput8:
		cfg = vpad.DInput8Config()
	case vpad.ProtocolXInput:
		cfg = vpad.DefaultConfig()
	default:
		logger.Fatalf("unknown -proto %q (supported: xinput, dinput8)", *proto)
	}
	if *name != "" {
		cfg.Name = *name
	}
	cfg.GrabPath = *grab
	cfg.Logger = logger

	if *profPath != "" {
		prof, err := profile.LoadFile(*profPath)
		if err != nil {
			logger.Fatalf("load profile %s: %v", *profPath, err)
		}
		cfg.Aim = prof.EffectiveAim()
		if cfg.Aim != nil && !*quiet {
			logger.Printf("loaded aim assist configuration from %s (curve=%s, filter=%s, gyro=%v, recoil=%v)",
				*profPath, cfg.Aim.Curve.Type, cfg.Aim.Filter.Type, cfg.Aim.Gyro.Enabled, cfg.Aim.AntiRecoil.Enabled)
		}
	}

	// Broadcast rumble events immediately (< 50 microseconds) with zero ticker delay
	sink := func(r vpad.Rumble) {
		srv.Broadcast(r)
	}

	pad, err := vpad.Open(cfg, sink)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	defer pad.Close()
	logger.Printf("virtual pad: %s", pad.Name())

	if !*quiet {
		logger.Printf("forwarding input from physical controller; serving FFB IPC on %s", endpoint)
	}
	if err := pad.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Fatalf("%v", err)
	}
}
