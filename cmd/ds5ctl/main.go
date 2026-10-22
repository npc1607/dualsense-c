// Command ds5ctl is the maintenance CLI: device discovery, profile checking,
// hardware self-test, effect benchmarking and input-report calibration.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	ossignal "os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/vpad/ipc"

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
	case "health":
		err = cmdHealth(os.Args[2:])
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
  health     query running ds5d daemon health and metrics
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
	jsonOut := fs.Bool("json", false, "output JSON format")
	_ = fs.Parse(args)

	builtin, err := profile.Builtin()
	if err != nil {
		return err
	}

	var custom []*profile.Profile
	if *dir != "" {
		c, err := profile.Dir(*dir)
		if err != nil {
			return err
		}
		custom = c
	}

	if *jsonOut {
		type profInfo struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Source      string `json:"source"`
		}
		var list []profInfo
		for _, p := range builtin {
			list = append(list, profInfo{Name: p.Name, Title: p.Title, Description: p.Description, Source: p.Source.Type})
		}
		for _, p := range custom {
			list = append(list, profInfo{Name: p.Name, Title: p.Title, Description: p.Description, Source: p.Source.Type})
		}
		out, _ := json.Marshal(list)
		fmt.Println(string(out))
		return nil
	}

	fmt.Println("builtin:")
	for _, p := range builtin {
		fmt.Printf("  %-10s %s\n", p.Name, p.Title)
	}
	if len(custom) > 0 {
		fmt.Printf("%s:\n", *dir)
		for _, p := range custom {
			fmt.Printf("  %-10s %s\n", p.Name, p.Title)
		}
	}
	return nil
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	name := fs.String("profile", "fps", "profile name or path")
	dir := fs.String("profiles", "", "profile directory")
	jsonOut := fs.Bool("json", false, "output JSON format")
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
	if *jsonOut {
		out, _ := json.Marshal(map[string]any{
			"ok":     true,
			"name":   p.Name,
			"rules":  len(p.Rules),
			"source": p.Source.Type,
		})
		fmt.Println(string(out))
		return nil
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

func tryDaemonRPC(ctx context.Context) (net.Conn, bool) {
	dialCtx, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
	defer cancel()
	conn, err := ipc.Dial(dialCtx, ipc.DefaultSignalEndpoint)
	if err != nil {
		return nil, false
	}
	_ = conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	if err := ipc.WriteCommand(conn, ipc.Command{ID: "probe", Method: "health"}); err != nil {
		_ = conn.Close()
		return nil, false
	}
	resp, err := ipc.ReadResponse(conn)
	_ = conn.SetDeadline(time.Time{})
	if err != nil || !resp.OK {
		_ = conn.Close()
		return nil, false
	}
	return conn, true
}

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	path := fs.String("hidraw", "", "device path")
	trigger := fs.String("trigger", "l2", "which trigger to exercise: l2, r2 or both")
	dur := fs.Duration("each", 2*time.Second, "time per effect")
	_ = fs.Parse(args)

	ctx := interruptContext()
	if conn, ok := tryDaemonRPC(ctx); ok {
		defer conn.Close()
		fmt.Printf("connected to active ds5d daemon on %s (single HID owner mode)\n", ipc.DefaultSignalEndpoint)
		for _, e := range benchEntries {
			fmt.Printf("-> %s\n", e.label)
			params := make(map[string]string)
			for i, val := range e.p {
				params[fmt.Sprintf("p%d", i)] = fmt.Sprintf("%d", val)
			}
			req := ipc.Command{
				ID:     "bench",
				Method: "effect.preview",
			}
			req.Params, _ = json.Marshal(map[string]any{
				"target": *trigger,
				"mode":   e.mode.String(),
				"params": params,
				"ttl_ms": int(dur.Milliseconds()),
			})
			_ = ipc.WriteCommand(conn, req)
			_, _ = ipc.ReadResponse(conn)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(*dur + 300*time.Millisecond):
			}
		}
		return nil
	}

	dev, err := openDevice(*path)
	if err != nil {
		return err
	}
	defer dev.Close()

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

func cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	endpoint := fs.String("endpoint", ipc.DefaultSignalEndpoint, "daemon IPC endpoint")
	jsonOut := fs.Bool("json", false, "output JSON format")
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := ipc.Dial(ctx, *endpoint)
	if err != nil {
		return fmt.Errorf("connect to ds5d at %s: %w", *endpoint, err)
	}
	defer conn.Close()

	if err := ipc.WriteCommand(conn, ipc.Command{ID: "health", Method: "health"}); err != nil {
		return err
	}
	resp, err := ipc.ReadResponse(conn)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("health error: %s", resp.Error)
	}

	if *jsonOut {
		fmt.Println(string(resp.Result))
		return nil
	}

	var data map[string]any
	_ = json.Unmarshal(resp.Result, &data)
	fmt.Printf("ds5d status at %s:\n", *endpoint)
	for k, v := range data {
		fmt.Printf("  %-14s %v\n", k+":", v)
	}
	return nil
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	path := fs.String("hidraw", "", "device path")
	_ = fs.Parse(args)

	ctx := interruptContext()
	if conn, ok := tryDaemonRPC(ctx); ok {
		defer conn.Close()
		fmt.Printf("connected to active ds5d daemon on %s (single HID owner mode)\n", ipc.DefaultSignalEndpoint)
		// Explicitly subscribe to STATE1 frames
		if err := ipc.WriteCommand(conn, ipc.Command{ID: "watch", Method: "state.subscribe"}); err != nil {
			return err
		}
		resp, err := ipc.ReadResponse(conn)
		if err != nil || !resp.OK {
			return fmt.Errorf("ds5d state subscribe failed: %v", err)
		}
		last := time.Now()
		for ctx.Err() == nil {
			hdr, sigs, _, summary, err := ipc.ReadStateFrame(conn)
			if err != nil {
				return err
			}
			if time.Since(last) < 100*time.Millisecond {
				continue
			}
			last = time.Now()
			var l2, r2, lx, ly, rx, ry float64
			for _, s := range sigs {
				switch s.Name {
				case "l2":
					l2 = s.Raw
				case "r2":
					r2 = s.Raw
				case "lx":
					lx = s.Raw
				case "ly":
					ly = s.Raw
				case "rx":
					rx = s.Raw
				case "ry":
					ry = s.Raw
				}
			}
			fmt.Printf("L2=%.2f R2=%.2f LX=%+.2f LY=%+.2f RX=%+.2f RY=%+.2f Rate=%dHz Seq=%d\n",
				l2, r2, lx, ly, rx, ry, summary.RateHz, hdr.Seq)
		}
		return nil
	}

	dev, err := openDevice(*path)
	if err != nil {
		return err
	}
	defer dev.Close()

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
		fmt.Printf("  gyro=%+.2f/%+.2f/%+.2f accel=%+.2f/%+.2f/%+.2f touch=%v id=%d (%.2f,%.2f) battery=%.1f%% raw=%#02x charging=%v\n",
			st.GyroX, st.GyroY, st.GyroZ, st.AccelX, st.AccelY, st.AccelZ,
			st.TouchActive, st.TouchID, st.TouchX, st.TouchY,
			st.Battery*100, st.BatteryRaw, st.Charging)
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
