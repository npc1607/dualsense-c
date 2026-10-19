// Command ds5d is the daemon: it loads a profile, runs the configured source
// adapter, evaluates the rules and drives the DualSense at a fixed tick rate.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	ossignal "os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/rules"
	sig "github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/sink"
	"github.com/npc1607/dualsense-c/internal/source"

	// register built-in sources
	_ "github.com/npc1607/dualsense-c/internal/source/all"
)

// version is injected at build time (-X main.version=...).
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("ds5d: %v", err)
	}
}

func run() error {
	var (
		profileName = flag.String("profile", "fps", "profile name (builtin) or path to a .yaml file")
		profilesDir = flag.String("profiles", "", "extra directory to look up profiles in")
		hidrawPath  = flag.String("hidraw", "", "force a device path (default: auto-detect)")
		rate        = flag.Int("rate", 125, "output reports per second")
		sourceType  = flag.String("source", "", "override the profile's source type (e.g. sim)")
		dryRun      = flag.Bool("dry-run", false, "do not open a controller; print frames instead")
		trace       = flag.Bool("trace", false, "log signal values twice per second")
		list        = flag.Bool("list", false, "list profiles and exit")
		showVer     = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("ds5d %s\n", version)
		return nil
	}

	logger := log.New(log.Writer(), "ds5d: ", log.LstdFlags)

	if *list {
		return listProfiles(*profilesDir)
	}

	p, err := profile.Resolve(*profileName, *profilesDir)
	if err != nil {
		return err
	}
	if *sourceType != "" {
		p.Source.Type = *sourceType
	}
	if err := p.Validate(); err != nil {
		return err
	}
	logger.Printf("profile %q (%s), source %q", p.Name, p.Title, p.Source.Type)

	shapers := make(map[string]sig.Shaper, len(p.Shape))
	for name, s := range p.Shape {
		shapers[name] = sig.Shaper{
			Deadzone: s.Deadzone,
			Gamma:    s.Gamma,
			SlewUp:   s.SlewUp,
			SlewDown: s.SlewDown,
		}
	}
	set := sig.NewSet(shapers)

	engine, err := rules.New(p, set, logger)
	if err != nil {
		return err
	}

	base, stop := ossignal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(base)
	defer cancel()

	var dev device.Device
	if !*dryRun {
		dev, err = openDevice(ctx, *hidrawPath, logger)
		if err != nil {
			return err
		}
		defer dev.Close()
		logger.Printf("controller: %s", dev.Info())
	}

	// Source -> signal store.
	frames := make(chan sig.Frame, 64)
	if p.Source.Type != "" {
		src, err := source.Open(p.Source, source.Deps{Device: dev, Logger: logger})
		if err != nil {
			return err
		}
		logger.Printf("source %q started", src.Name())
		go func() {
			if err := src.Run(ctx, frames); err != nil && ctx.Err() == nil {
				logger.Printf("source %q stopped: %v", src.Name(), err)
				cancel()
			}
		}()
	}
	go func() {
		for f := range frames {
			set.Apply(f)
		}
	}()

	if *trace {
		go traceSignals(ctx, set, p, logger)
	}

	statusBar := p.StatusLight() && !p.HasTarget("lightbar")
	produce := func(now time.Time) hid.Frame {
		f := engine.Step(now)
		if statusBar {
			if set.Fresh(now, p.TTL()) {
				f.Lightbar = [3]uint8{0, 255, 0} // live
			} else {
				f.Lightbar = [3]uint8{255, 200, 0} // idle
			}
		}
		return f
	}

	if *dryRun {
		logger.Printf("dry run: printing frames at 10 Hz (ctrl-c to stop)")
		return runDry(ctx, produce, logger)
	}

	logger.Printf("driving %s at %d Hz", dev.Info().Path, *rate)
	s := sink.New(dev, *rate, logger)
	// Survive a USB re-enumeration: the hidraw node changes number when the
	// controller is unplugged or the link resets, so re-find it instead of
	// writing into a dead fd.
	s.SetReopen(func(old device.Device) (device.Device, error) {
		_ = old.Close()
		if *hidrawPath != "" {
			if d, err := device.OpenPath(*hidrawPath); err == nil {
				return d, nil
			}
		}
		info, err := device.Find()
		if err != nil {
			return nil, err
		}
		return device.Open(info)
	})
	return s.Run(ctx, produce)
}

func openDevice(ctx context.Context, path string, logger *log.Logger) (device.Device, error) {
	if path != "" {
		return device.OpenPath(path)
	}
	for {
		info, err := device.Find()
		if err == nil {
			return device.Open(info)
		}
		if errors.Is(err, device.ErrUnsupported) {
			return nil, fmt.Errorf("%w (use -dry-run to test profiles)", err)
		}
		logger.Printf("%v — retrying in 3s", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func runDry(ctx context.Context, produce sink.Producer, logger *log.Logger) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			f := produce(now)
			logger.Printf("frame l2=%s r2=%s rumble=%d/%d bar=%d,%d,%d",
				trigger(f.L2), trigger(f.R2), f.RumbleL, f.RumbleR,
				f.Lightbar[0], f.Lightbar[1], f.Lightbar[2])
		}
	}
}

func trigger(t hid.Trigger) string {
	if t.IsOff() {
		return "off"
	}
	return fmt.Sprintf("%s[%d %d %d]", t.Mode, t.P[0], t.P[1], t.P[2])
}

func traceSignals(ctx context.Context, set *sig.Set, p *profile.Profile, logger *log.Logger) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			vals, _ := set.Snapshot(now, p.TTL())
			if len(vals) == 0 {
				logger.Printf("signals: (none)")
				continue
			}
			names := make([]string, 0, len(vals))
			for n := range vals {
				names = append(names, n)
			}
			sort.Strings(names)
			var b strings.Builder
			for _, n := range names {
				fmt.Fprintf(&b, "%s=%.3f ", n, vals[n])
			}
			logger.Printf("signals: %s", strings.TrimSpace(b.String()))
		}
	}
}

func listProfiles(dir string) error {
	builtin, err := profile.Builtin()
	if err != nil {
		return err
	}
	fmt.Printf("builtin profiles:\n")
	for _, p := range builtin {
		fmt.Printf("  %-10s %s\n", p.Name, p.Title)
	}
	if dir == "" {
		return nil
	}
	custom, err := profile.Dir(dir)
	if err != nil {
		return err
	}
	fmt.Printf("profiles in %s:\n", dir)
	for _, p := range custom {
		fmt.Printf("  %-10s %s\n", p.Name, p.Title)
	}
	return nil
}
