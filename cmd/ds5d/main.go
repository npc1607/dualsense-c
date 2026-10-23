// Command ds5d is the daemon: it loads a profile, runs the configured source
// adapter, evaluates the rules and drives the DualSense at a fixed tick rate.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	ossignal "os/signal"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/radio"
	"github.com/npc1607/dualsense-c/internal/rules"
	sig "github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/sink"
	"github.com/npc1607/dualsense-c/internal/sink/haptics"
	"github.com/npc1607/dualsense-c/internal/source"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"

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
		profileName  = flag.String("profile", "fps", "profile name (builtin) or path to a .yaml file")
		profilesDir  = flag.String("profiles", "", "extra directory to look up profiles in")
		hidrawPath   = flag.String("hidraw", "", "force a device path (default: auto-detect)")
		rate         = flag.Int("rate", 125, "output reports per second")
		sourceType   = flag.String("source", "", "override the profile's source type (e.g. sim)")
		dryRun       = flag.Bool("dry-run", false, "do not open a controller; print frames instead")
		trace        = flag.Bool("trace", false, "log signal values twice per second")
		list         = flag.Bool("list", false, "list profiles and exit")
		ipcBroadcast = flag.String("ipc-broadcast", "127.0.0.1:9997", "IPC telemetry broadcast endpoint (empty to disable)")
		showVer      = flag.Bool("version", false, "print version and exit")
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

	// profileMu guards the active profile pointer: profile.reload swaps it from
	// an IPC goroutine while the STATE1 ticker, the sink loop and the other
	// CMD1 handlers read it. Loaded profiles are treated as immutable, so the
	// accessor may hand out the pointer itself.
	var profileMu sync.RWMutex
	currentProfile := func() *profile.Profile {
		profileMu.RLock()
		defer profileMu.RUnlock()
		return p
	}
	setProfile := func(np *profile.Profile) {
		profileMu.Lock()
		p = np
		profileMu.Unlock()
	}

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

	var sigServer *ipc.Server

	player, err := radio.NewPlayer(radio.Config{Logger: logger})
	if err != nil {
		logger.Printf("radio: player init: %v", err)
	}

	speechCh := make(chan rules.SpeechEvent, 32)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-speechCh:
				if player != nil && ev.Clip != "" {
					_ = player.PlayWithPriority(ctx, ev.Clip, ev.Priority)
				}
			}
		}
	}()

	engine.SetSpeechHandler(func(ev rules.SpeechEvent) {
		select {
		case speechCh <- ev:
		default:
		}
		if sigServer != nil {
			sigServer.BroadcastSpeech(ipc.SpeechFrame{
				Name:     ev.Name,
				Target:   ev.Target,
				Clip:     ev.Clip,
				Priority: ev.Priority,
				Text:     ev.Text,
				At:       ev.At,
			})
		}
	})

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
	if *ipcBroadcast != "" {
		srv, err := ipc.Listen(*ipcBroadcast, logger)
		if err != nil {
			logger.Printf("warning: ipc broadcast disabled: %v", err)
		} else {
			sigServer = srv
			defer sigServer.Close()
			logger.Printf("ipc telemetry broadcaster active on %s", sigServer.Endpoint())
		}
	}

	go func() {
		for f := range frames {
			set.Apply(f)
			if sigServer != nil && len(f.Values) > 0 {
				sigServer.BroadcastSignals(f.Values)
			}
		}
	}()

	if *trace {
		go traceSignals(ctx, set, func() time.Duration { return currentProfile().TTL() }, logger)
	}

	startTime := time.Now()
	var (
		lastFrameMu    sync.RWMutex
		lastFrame      hid.Frame
		lastTick       time.Time
		tickJitterUS   uint32
		previewMu      sync.RWMutex
		previewTarget  string
		previewTrigger hid.Trigger
		previewExpire  time.Time
	)

	stateTicker := time.NewTicker(time.Second / 30) // default 30Hz
	defer stateTicker.Stop()

	go func() {
		stateSeq := uint32(0)
		for {
			select {
			case <-ctx.Done():
				return
			case tNow := <-stateTicker.C:
				if sigServer == nil || sigServer.Subscribers() == 0 {
					continue
				}
				stateSeq++
				sigTelem := set.Telemetry(tNow, currentProfile().TTL())
				ruleTelem := engine.Telemetry(tNow)

				lastFrameMu.RLock()
				curF := lastFrame
				jitter := tickJitterUS
				lastFrameMu.RUnlock()

				muteByte := uint8(0)
				if curF.MuteLED {
					muteByte = 1
				}
				summary := ipc.StateSummary{
					L2Mode:       uint8(curF.L2.Mode),
					L2Params:     [4]uint8{curF.L2.P[0], curF.L2.P[1], curF.L2.P[2], curF.L2.P[3]},
					R2Mode:       uint8(curF.R2.Mode),
					R2Params:     [4]uint8{curF.R2.P[0], curF.R2.P[1], curF.R2.P[2], curF.R2.P[3]},
					RumbleLeft:   curF.RumbleL,
					RumbleRight:  curF.RumbleR,
					LightbarR:    curF.Lightbar[0],
					LightbarG:    curF.Lightbar[1],
					LightbarB:    curF.Lightbar[2],
					PlayerLEDs:   curF.PlayerLEDs,
					MuteLED:      muteByte,
					HapticLeft:   curF.HapticL,
					HapticRight:  curF.HapticR,
					RateHz:       uint16(*rate),
					TickJitterUS: jitter,
					Subscribers:  uint16(sigServer.Subscribers()),
				}

				hdr := ipc.StateHeader{
					Flags:  0,
					Seq:    stateSeq,
					TickMS: uint32(tNow.Sub(startTime).Milliseconds()),
				}
				frame := ipc.EncodeStateFrame(hdr, sigTelem, ruleTelem, summary)
				sigServer.BroadcastState(frame)
			}
		}
	}()

	if sigServer != nil {
		sigServer.SetCommandHandler(func(cmd ipc.Command) ipc.Response {
			switch cmd.Method {
			case "health":
				cur := currentProfile()
				res, _ := json.Marshal(map[string]any{
					"uptime_s":    time.Since(startTime).Seconds(),
					"subscribers": sigServer.Subscribers(),
					"rate_hz":     *rate,
					"profile":     cur.Name,
					"source":      cur.Source.Type,
					"jitter_us":   tickJitterUS,
				})
				return ipc.Response{ID: cmd.ID, OK: true, Result: res}

			case "state.subscribe":
				var params struct {
					Rate int `json:"rate"`
				}
				_ = json.Unmarshal(cmd.Params, &params)
				if params.Rate == 30 || params.Rate == 60 {
					stateTicker.Reset(time.Second / time.Duration(params.Rate))
				}
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"subscribed":true}`)}

			case "signals.subscribe":
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"subscribed":true}`)}

			case "profile.reload":
				newP, err := profile.Resolve(*profileName, *profilesDir)
				if err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				if err := newP.Validate(); err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				newShapers := make(map[string]sig.Shaper, len(newP.Shape))
				for name, s := range newP.Shape {
					newShapers[name] = sig.Shaper{
						Deadzone: s.Deadzone,
						Gamma:    s.Gamma,
						SlewUp:   s.SlewUp,
						SlewDown: s.SlewDown,
					}
				}
				set.SetShapers(newShapers)
				if err := engine.Reload(newP); err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				setProfile(newP)
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"reloaded":true}`)}

			case "profile.apply":
				applied, ignored, reasons, err := applyShapePatch(currentProfile().Extra, set, cmd.Params)
				if err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				res, _ := json.Marshal(map[string]any{
					"applied": applied,
					"ignored": ignored,
					"reasons": reasons,
				})
				return ipc.Response{ID: cmd.ID, OK: true, Result: res}

			case "effect.preview":
				var params struct {
					Target string            `json:"target"`
					Mode   string            `json:"mode"`
					Params map[string]string `json:"params"`
					TTLMS  int               `json:"ttl_ms"`
				}
				if err := json.Unmarshal(cmd.Params, &params); err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				ttl := time.Duration(params.TTLMS) * time.Millisecond
				if ttl <= 0 {
					ttl = 1000 * time.Millisecond
				}
				m, ok := hid.ParseMode(params.Mode)
				if !ok {
					return ipc.Response{ID: cmd.ID, OK: false, Error: fmt.Sprintf("unknown mode %q", params.Mode)}
				}
				var tr hid.Trigger
				tr.Mode = m
				for k, v := range params.Params {
					if idx, ok := hid.ParamIndex(m, k); ok {
						if num, err := strconv.Atoi(v); err == nil {
							if idx < len(tr.P) {
								tr.P[idx] = uint8(num)
							}
						}
					}
				}
				previewMu.Lock()
				previewTarget = params.Target
				previewTrigger = tr
				previewExpire = time.Now().Add(ttl)
				previewMu.Unlock()
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"preview":true}`)}

			case "rules.simulate":
				var params struct {
					Signals map[string]float64 `json:"signals"`
				}
				if err := json.Unmarshal(cmd.Params, &params); err != nil {
					return ipc.Response{ID: cmd.ID, OK: false, Error: err.Error()}
				}
				frame, ruleTelem := engine.Simulate(time.Now(), params.Signals)
				res, _ := json.Marshal(map[string]any{
					"frame": frame,
					"rules": ruleTelem,
				})
				return ipc.Response{ID: cmd.ID, OK: true, Result: res}

			case "hid.claim", "hid.release":
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"claimed":true}`)}

			case "rate.set":
				var params struct {
					Hz int `json:"hz"`
				}
				if err := json.Unmarshal(cmd.Params, &params); err == nil && params.Hz > 0 {
					*rate = params.Hz
				}
				return ipc.Response{ID: cmd.ID, OK: true, Result: json.RawMessage(`{"rate_set":true}`)}

			default:
				return ipc.Response{ID: cmd.ID, OK: false, Error: fmt.Sprintf("unknown method %q", cmd.Method)}
			}
		})
	}

	statusBar := p.StatusLight() && !p.HasTarget("lightbar")
	expectedInterval := time.Second / time.Duration(*rate)
	produce := func(now time.Time) hid.Frame {
		if !lastTick.IsZero() {
			jitter := now.Sub(lastTick) - expectedInterval
			if jitter < 0 {
				jitter = -jitter
			}
			tickJitterUS = uint32(jitter.Microseconds())
		}
		lastTick = now

		f := engine.Step(now)

		previewMu.RLock()
		if now.Before(previewExpire) {
			if previewTarget == "l2" || previewTarget == "both" {
				f.L2 = previewTrigger
			}
			if previewTarget == "r2" || previewTarget == "both" {
				f.R2 = previewTrigger
			}
		}
		previewMu.RUnlock()

		if statusBar {
			if set.Fresh(now, currentProfile().TTL()) {
				f.Lightbar = [3]uint8{0, 255, 0} // live
			} else {
				f.Lightbar = [3]uint8{255, 200, 0} // idle
			}
		}

		lastFrameMu.Lock()
		lastFrame = f
		lastFrameMu.Unlock()
		return f
	}

	if *dryRun {
		logger.Printf("dry run: printing frames at 10 Hz (ctrl-c to stop)")
		return runDry(ctx, produce, logger)
	}

	// Optional voice-coil output over the controller's USB audio interface.
	if p.Haptics.IsEnabled() {
		hw, err := haptics.Open(haptics.Config{
			Target:    p.Haptics.Target,
			CarrierHz: p.Haptics.CarrierHz,
			Gain:      p.Haptics.Gain,
			Command:   p.Haptics.Command,
			Mode:      p.Haptics.Mode,
			Source:    p.Haptics.Source,
			LowHz:     p.Haptics.LowHz,
			HighHz:    p.Haptics.HighHz,
		}, logger)
		if err != nil {
			logger.Printf("haptics disabled: %v", err)
		} else {
			defer hw.Close()
			go func() {
				if err := hw.Run(ctx); err != nil && ctx.Err() == nil {
					logger.Printf("haptics stopped: %v", err)
				}
			}()
			inner := produce
			produce = func(now time.Time) hid.Frame {
				f := inner(now)
				hw.Set(f.HapticL, f.HapticR)
				return f
			}
			logger.Printf("haptics: voice-coil output enabled")
		}
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

// shapeFieldPatch uses pointers so we can distinguish an omitted field from an
// explicit zero (needed to reject gamma:0 while still allowing gamma to be
// omitted).
type shapeFieldPatch struct {
	Deadzone *float64 `json:"deadzone"`
	Gamma    *float64 `json:"gamma"`
	SlewUp   *float64 `json:"slew_up"`
	SlewDown *float64 `json:"slew_down"`
}

// validateShapePatch checks only the fields the request explicitly set, so a
// profile's "zero means disabled" values are never mistaken for out-of-range.
// Reasons are returned in a fixed field order for predictable responses.
func validateShapePatch(fp shapeFieldPatch, sh sig.Shaper) string {
	if fp.Deadzone != nil && (sh.Deadzone < 0 || sh.Deadzone > 1) {
		return "deadzone must be between 0 and 1"
	}
	if fp.Gamma != nil && sh.Gamma <= 0 {
		return "gamma must be > 0"
	}
	if fp.SlewUp != nil && sh.SlewUp < 0 {
		return "slew_up must be >= 0"
	}
	if fp.SlewDown != nil && sh.SlewDown < 0 {
		return "slew_down must be >= 0"
	}
	return ""
}

// applyShapePatch merges a partial shape patch into the live signal set and
// reports per-key results.
//
// The merge base is the current runtime shaper (initialised from p.Shape and
// replaced wholesale by profile.reload), so patches are runtime-only:
// profile.reload re-reads the YAML profile and intentionally discards them.
// We avoid writing back into p.Shape so the handler never mutates a map shared
// with engine.Simulate / Engine.Reload.
func applyShapePatch(
	extra []string,
	set *sig.Set,
	raw json.RawMessage,
) (applied, ignored []string, reasons map[string]string, err error) {
	if len(raw) == 0 {
		return nil, nil, nil, fmt.Errorf("params.shape is required")
	}

	var params struct {
		Shape map[string]shapeFieldPatch `json:"shape"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid params: %w", err)
	}
	if len(params.Shape) == 0 {
		return nil, nil, nil, fmt.Errorf("params.shape must contain at least one signal")
	}

	ignored = []string{}
	reasons = make(map[string]string)
	patch := make(map[string]sig.Shaper, len(params.Shape))

	for name, fp := range params.Shape {
		if !sig.IsStandard(name) && !slices.Contains(extra, name) {
			ignored = append(ignored, name)
			reasons[name] = "unknown signal"
			continue
		}

		// A field name we do not know (a typo, or the pre-snake_case spelling)
		// decodes to all-nil pointers, so reject it instead of reporting the
		// signal as applied while nothing changed.
		if fp.Deadzone == nil && fp.Gamma == nil && fp.SlewUp == nil && fp.SlewDown == nil {
			ignored = append(ignored, name)
			reasons[name] = "no recognised fields"
			continue
		}

		sh, _ := set.GetShaper(name)
		if fp.Deadzone != nil {
			sh.Deadzone = *fp.Deadzone
		}
		if fp.Gamma != nil {
			sh.Gamma = *fp.Gamma
		}
		if fp.SlewUp != nil {
			sh.SlewUp = *fp.SlewUp
		}
		if fp.SlewDown != nil {
			sh.SlewDown = *fp.SlewDown
		}
		if reason := validateShapePatch(fp, sh); reason != "" {
			ignored = append(ignored, name)
			reasons[name] = reason
			continue
		}
		patch[name] = sh
	}

	applied = set.MergeShapers(patch)
	sort.Strings(ignored)
	return applied, ignored, reasons, nil
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

// traceSignals logs signal values twice per second.
//
// ttl is a function so a profile.reload mid-trace takes effect immediately.
func traceSignals(ctx context.Context, set *sig.Set, ttl func() time.Duration, logger *log.Logger) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			vals, _ := set.Snapshot(now, ttl())
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
