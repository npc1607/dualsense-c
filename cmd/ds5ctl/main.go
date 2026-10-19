// Command ds5ctl is the maintenance CLI: device discovery, profile checking,
// hardware self-test, effect benchmarking and input-report calibration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	ossignal "os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/input"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/report"
	"github.com/npc1607/dualsense-c/internal/rules"
	sig "github.com/npc1607/dualsense-c/internal/signal"

	_ "github.com/npc1607/dualsense-c/internal/source/all"
)

// version is injected at build time (-X main.version=...).
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "list":
		err = cmdList(os.Args[2:])
	case "profiles":
		err = cmdProfiles(os.Args[2:])
	case "check":
		err = cmdCheck(os.Args[2:])
	case "test":
		err = cmdTest(os.Args[2:])
	case "bench":
		err = cmdBench(os.Args[2:])
	case "watch":
		err = cmdWatch(os.Args[2:])
	case "sniff":
		err = cmdSniff(os.Args[2:])
	case "version", "-version", "--version":
		fmt.Printf("ds5ctl %s\n", version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ds5ctl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: ds5ctl <command> [flags]

  list       enumerate connected DualSense controllers
  profiles   list builtin profiles (and a profile directory)
  check      compile a profile's rules without touching hardware
  test       rumble + trigger ramp self-test
  bench      cycle every trigger effect so you can pick the right one
  watch      print decoded input reports (calibrates button bits)
  sniff      hexdump incoming game telemetry (calibrates UDP offsets)
  version    print the build version
`)
}

func cmdList(args []string) error {
	_ = flag.NewFlagSet("list", flag.ExitOnError).Parse(args)
	all := device.FindAll()
	if len(all) == 0 {
		fmt.Println("no DualSense found.")
		fmt.Println("  * plug it in over USB (Bluetooth adaptive triggers are unreliable)")
		fmt.Println("  * Linux: install udev/70-dualsense.rules so /dev/hidraw* is writable")
		return nil
	}
	for _, d := range all {
		fmt.Println(d)
	}
	return nil
}

func cmdProfiles(args []string) error {
	fs := flag.NewFlagSet("profiles", flag.ExitOnError)
	dir := fs.String("dir", "", "also list profiles in this directory")
	_ = fs.Parse(args)

	builtin, err := profile.Builtin()
	if err != nil {
		return err
	}
	fmt.Println("builtin:")
	for _, p := range builtin {
		fmt.Printf("  %-10s %s\n", p.Name, p.Title)
	}
	if *dir == "" {
		return nil
	}
	custom, err := profile.Dir(*dir)
	if err != nil {
		return err
	}
	fmt.Printf("%s:\n", *dir)
	for _, p := range custom {
		fmt.Printf("  %-10s %s\n", p.Name, p.Title)
	}
	return nil
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	name := fs.String("profile", "fps", "profile name or path")
	dir := fs.String("profiles", "", "profile directory")
	_ = fs.Parse(args)

	p, err := profile.Resolve(*name, *dir)
	if err != nil {
		return err
	}
	set := sig.NewSet(nil)
	logger := log.New(os.Stderr, "check: ", 0)
	if _, err := rules.New(p, set, logger); err != nil {
		return err
	}
	fmt.Printf("ok: %s (%d rules, source %q)\n", p.Name, len(p.Rules), p.Source.Type)
	return nil
}

func cmdTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	path := fs.String("hidraw", "", "device path")
	_ = fs.Parse(args)

	dev, err := openDevice(*path)
	if err != nil {
		return err
	}
	defer dev.Close()
	fmt.Printf("device: %s\n", dev.Info())

	ctx := interruptContext()
	write := func(f hid.Frame) { _ = dev.Write(report.Build(dev.Info(), f)) }
	tick := time.NewTicker(8 * time.Millisecond)
	defer tick.Stop()

	ramp := func(label string, dur time.Duration, fn func(t float64) hid.Frame) error {
		fmt.Println("->", label)
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case now := <-tick.C:
				k := now.Sub(start).Seconds() / dur.Seconds()
				if k >= 1 {
					return nil
				}
				write(fn(k))
			}
		}
	}
	defer write(hid.Idle())

	if err := ramp("rumble ramp (left+right)", 2*time.Second, func(t float64) hid.Frame {
		v := uint8(t * 255)
		return hid.Frame{RumbleL: v, RumbleR: v}
	}); err != nil {
		return nil
	}
	if err := ramp("L2 rigid ramp", 2*time.Second, func(t float64) hid.Frame {
		tr := hid.Trigger{Mode: hid.ModeRigid}
		tr.Set(0, 0)
		tr.Set(1, t*255)
		return hid.Frame{L2: tr}
	}); err != nil {
		return nil
	}
	if err := ramp("R2 rigid ramp", 2*time.Second, func(t float64) hid.Frame {
		tr := hid.Trigger{Mode: hid.ModeRigid}
		tr.Set(0, 0)
		tr.Set(1, t*255)
		return hid.Frame{R2: tr}
	}); err != nil {
		return nil
	}
	if err := ramp("L2 feedback hold", 1500*time.Millisecond, func(t float64) hid.Frame {
		tr := hid.Trigger{Mode: hid.ModeFeedback}
		tr.Set(0, 0)
		tr.Set(1, 150)
		tr.Set(2, 60+t*180)
		return hid.Frame{L2: tr, Lightbar: [3]uint8{0, 0, 255}}
	}); err != nil {
		return nil
	}
	fmt.Println("done")
	return nil
}

type benchEntry struct {
	label string
	mode  hid.Mode
	p     [10]byte
}

var benchEntries = []benchEntry{
	{"rigid(start=0 force=180)", hid.ModeRigid, [10]byte{0, 180}},
	{"section(start=2 end=7)", hid.ModeSection, [10]byte{2, 7}},
	{"feedback(start=0 strength=150 force=120)", hid.ModeFeedback, [10]byte{0, 150, 120}},
	{"weapon(start=0 ampA=60 ampB=140 freq=8)", hid.ModeWeapon, [10]byte{0, 60, 140, 8}},
	{"vibration(pos=3 amp=200 freq=20)", hid.ModeVibration, [10]byte{3, 200, 20}},
	{"slope_feedback(0..9 40->200)", hid.ModeSlopeFeedback, [10]byte{0, 9, 40, 200}},
	{"slope_weapon(0..9 40->200 freq=8)", hid.ModeSlopeWeapon, [10]byte{0, 9, 40, 200, 8}},
	{"machine(2..6 freq=20 period=5)", hid.ModeMachine, [10]byte{2, 6, 20, 5}},
}

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	path := fs.String("hidraw", "", "device path")
	trigger := fs.String("trigger", "l2", "which trigger to exercise: l2, r2 or both")
	dur := fs.Duration("each", 2*time.Second, "time per effect")
	_ = fs.Parse(args)

	dev, err := openDevice(*path)
	if err != nil {
		return err
	}
	defer dev.Close()

	ctx := interruptContext()
	tick := time.NewTicker(8 * time.Millisecond)
	defer tick.Stop()
	write := func(f hid.Frame) { _ = dev.Write(report.Build(dev.Info(), f)) }
	defer write(hid.Idle())

	fmt.Printf("device: %s (ctrl-c to stop)\n", dev.Info())
	for _, e := range benchEntries {
		fmt.Printf("-> %s\n", e.label)
		tr := hid.Trigger{Mode: e.mode, P: e.p}
		f := hid.Frame{Lightbar: [3]uint8{0, 80, 255}}
		switch *trigger {
		case "r2":
			f.R2 = tr
		case "both":
			f.L2, f.R2 = tr, tr
		default:
			f.L2 = tr
		}
		start := time.Now()
	inner:
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-tick.C:
				if time.Since(start) >= *dur {
					break inner
				}
				write(f)
			}
		}
		write(hid.Frame{}) // release before the next effect
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	path := fs.String("hidraw", "", "device path")
	_ = fs.Parse(args)

	dev, err := openDevice(*path)
	if err != nil {
		return err
	}
	defer dev.Close()

	ctx := interruptContext()
	buf := make([]byte, 128)
	last := time.Now()
	fmt.Printf("watching %s (ctrl-c to stop)\n", dev.Info().Path)
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := dev.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		st, ok := input.Parse(buf[:n])
		if !ok || time.Since(last) < 100*time.Millisecond {
			continue
		}
		last = time.Now()
		fmt.Printf("L2=%.2f R2=%.2f LX=%+.2f LY=%+.2f RX=%+.2f RY=%+.2f L1=%v R1=%v cross=%v seq=%d\n",
			st.L2, st.R2, st.LX, st.LY, st.RX, st.RY, st.L1, st.R1, st.Cross, st.Sequence)
	}
}

// cmdSniff prints incoming telemetry datagrams so a game's UDP layout can be
// calibrated from the field table in a profile, without touching Go code.
func cmdSniff(args []string) error {
	fs := flag.NewFlagSet("sniff", flag.ExitOnError)
	listen := fs.String("listen", "0.0.0.0:20777", "UDP address to listen on")
	width := fs.Int("hex", 64, "bytes to print per packet")
	every := fs.Duration("every", 500*time.Millisecond, "min interval between dumps of the same packet length")
	_ = fs.Parse(args)

	addr, err := net.ResolveUDPAddr("udp4", *listen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx := interruptContext()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	fmt.Printf("listening on %s (ctrl-c to stop)\n", conn.LocalAddr())
	fmt.Println("trigger the game so it sends telemetry, then read the moving bytes off the dump")

	last := map[int]time.Time{}
	buf := make([]byte, 4096)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if time.Since(last[n]) < *every {
			continue
		}
		last[n] = time.Now()
		fmt.Printf("\n--- len=%d from %s at %s ---\n%s\n",
			n, from, time.Now().Format("15:04:05.000"), hexDump(buf[:n], *width))
	}
}

func hexDump(b []byte, limit int) string {
	if limit > 0 && len(b) > limit {
		b = b[:limit]
	}
	const hex = "0123456789abcdef"
	var sb strings.Builder
	for i, c := range b {
		if i%16 == 0 {
			fmt.Fprintf(&sb, "%04x  ", i)
		}
		sb.WriteByte(hex[c>>4])
		sb.WriteByte(hex[c&0xf])
		sb.WriteByte(' ')
		if i%16 == 15 {
			sb.WriteByte('\n')
		}
	}
	if len(b)%16 != 0 {
		sb.WriteByte('\n')
	}
	return sb.String()
}

func openDevice(path string) (device.Device, error) {
	if path != "" {
		return device.OpenPath(path)
	}
	info, err := device.Find()
	if err != nil {
		return nil, err
	}
	return device.Open(info)
}

func interruptContext() context.Context {
	ctx, _ := ossignal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	return ctx
}
