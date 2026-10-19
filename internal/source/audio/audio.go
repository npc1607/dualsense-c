// Package audio derives haptic-relevant signals from what the system is
// already playing: a game's engine note, tyre scrub, gunshots and explosions
// all carry energy in the 20-400 Hz band, which is exactly what the DualSense
// voice coils reproduce well.
//
// It does not need any game support: it taps a PipeWire sink monitor and turns
// the low-frequency envelope into the `bass`, `mid` and `loudness` signals.
//
// Audio is captured by a child process (pw-record by default) so the package
// stays pure Go: no cgo, no PipeWire headers, and the same code works on any
// distro that ships the PipeWire CLI tools.
package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("audio", New) }

// defaults, all overridable from the profile's source block.
const (
	defCommand   = "pw-record --target={target} --rate={rate} --channels={channels} --format={format} -"
	defRate      = 24000
	defChannels  = 2
	defFormat    = "f32"
	defEmitHz    = 60
	defBassLow   = 20.0
	defBassHigh  = 110.0
	defMidLow    = 160.0
	defMidHigh   = 450.0
	defGain      = 2.5
	defAttackMS  = 4.0
	defReleaseMS = 90.0
)

// Source turns a PipeWire monitor into envelope signals.
type Source struct {
	cmd          []string
	rate         int
	channels     int
	emitHz       int
	gain         float64
	bassEnv      *envFollower
	midEnv       *envFollower
	loudEnv      *envFollower
	bassL, bassH cascade
	midL, midH   cascade
	log          *log.Logger
}

// New builds the source from the profile's `source:` block.
//
// Recognised parameters (all optional):
//
//	target       PipeWire sink or monitor name to record from (required)
//	command      capture command template, {target}/{rate}/{channels}/{format}
//	rate         capture sample rate (default 24000)
//	channels     capture channel count (default 2)
//	format       sample format passed to the command (default f32)
//	gain         linear gain applied to the envelope (default 2.5)
//	attack_ms    envelope attack time (default 4)
//	release_ms   envelope release time (default 90)
//	bass_hz      bass band edges, "low,high" (default 20,110)
//	mid_hz       mid band edges, "low,high" (default 160,450)
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	getS := func(k, def string) string {
		if v, ok := cfg.Params[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
		return def
	}
	getF := func(k string, def float64) float64 {
		if v, ok := cfg.Params[k]; ok {
			switch n := v.(type) {
			case float64:
				return n
			case int:
				return float64(n)
			}
		}
		return def
	}
	getI := func(k string, def int) int {
		return int(getF(k, float64(def)))
	}

	target := getS("target", "")
	if target == "" {
		return nil, errors.New(`audio source: "target" is required (a sink/monitor name, or "auto")`)
	}
	if target == "auto" {
		t, err := detectMonitor()
		if err != nil {
			return nil, err
		}
		target = t
	}

	rate := getI("rate", defRate)
	channels := getI("channels", defChannels)
	if rate <= 0 || channels <= 0 {
		return nil, errors.New("audio source: rate and channels must be positive")
	}
	format := getS("format", defFormat)

	tmpl := getS("command", defCommand)
	repl := strings.NewReplacer(
		"{target}", target,
		"{rate}", fmt.Sprint(rate),
		"{channels}", fmt.Sprint(channels),
		"{format}", format,
	)
	cmd := strings.Fields(repl.Replace(tmpl))
	if len(cmd) == 0 {
		return nil, errors.New("audio source: empty command")
	}

	bassLo, bassHi := parseBand(getS("bass_hz", ""), defBassLow, defBassHigh)
	midLo, midHi := parseBand(getS("mid_hz", ""), defMidLow, defMidHigh)
	if bassHi >= float64(rate)/2 || midHi >= float64(rate)/2 {
		return nil, fmt.Errorf("audio source: band edge above Nyquist for %d Hz capture (raise rate)", rate)
	}

	logger := deps.Logger
	if logger == nil {
		logger = log.Default()
	}

	attackMS := getF("attack_ms", defAttackMS)
	releaseMS := getF("release_ms", defReleaseMS)
	if attackMS <= 0 {
		attackMS = defAttackMS
	}
	if releaseMS <= 0 {
		releaseMS = defReleaseMS
	}

	s := &Source{
		cmd:      cmd,
		rate:     rate,
		channels: channels,
		emitHz:   getI("rate_hz", defEmitHz),
		gain:     getF("gain", defGain),
		bassL:    newHighPassCascade(bassLo, float64(rate)),
		bassH:    newLowPassCascade(bassHi, float64(rate)),
		midL:     newHighPassCascade(midLo, float64(rate)),
		midH:     newLowPassCascade(midHi, float64(rate)),
		log:      logger,
	}
	if s.emitHz <= 0 {
		s.emitHz = defEmitHz
	}
	s.bassEnv = newEnvFollower(attackMS, releaseMS, float64(rate))
	s.midEnv = newEnvFollower(attackMS, releaseMS, float64(rate))
	s.loudEnv = newEnvFollower(attackMS, releaseMS, float64(rate))
	return s, nil
}

func parseBand(spec string, defLo, defHi float64) (float64, float64) {
	lo, hi := defLo, defHi
	parts := strings.Split(spec, ",")
	if len(parts) == 2 {
		var a, b float64
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%g", &a); err == nil {
			lo = a
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%g", &b); err == nil {
			hi = b
		}
	}
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		hi = lo * 2
	}
	return lo, hi
}

func (s *Source) Name() string { return "audio" }

// Run spawns the capture command and processes its PCM stream until ctx ends.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	cmd := exec.CommandContext(ctx, s.cmd[0], s.cmd[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("audio: start %s: %w", s.cmd[0], err)
	}
	s.log.Printf("audio: capturing %s (%s %dch %d Hz)", strings.Join(s.cmd, " "), s.cmd[0], s.channels, s.rate)

	// Surface the capture tool's diagnostics instead of swallowing them.
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				s.log.Printf("audio[%s]: %s", s.cmd[0], line)
			}
		}
	}()

	err = s.consume(ctx, stdout, out)
	_ = cmd.Process.Kill()
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	if werr != nil {
		return fmt.Errorf("audio: capture command exited: %w", werr)
	}
	return io.EOF
}

// consume decodes f32le PCM, filters it into bands and emits envelope frames.
func (s *Source) consume(ctx context.Context, r io.Reader, out chan<- signal.Frame) error {
	const block = 1024 // frames per read
	raw := make([]byte, block*s.channels*4)
	sample := make([]float32, 0, block)
	emit := time.Second / time.Duration(s.emitHz)
	next := time.Now()

	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := io.ReadFull(r, raw)
		if err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				return nil
			}
			return fmt.Errorf("audio: read: %w", err)
		}
		frames := n / (4 * s.channels)
		sample = sample[:0]
		for i := 0; i < frames; i++ {
			var sum float32
			for c := 0; c < s.channels; c++ {
				off := (i*s.channels + c) * 4
				sum += math.Float32frombits(binary.LittleEndian.Uint32(raw[off:]))
			}
			sample = append(sample, sum/float32(s.channels))
		}

		bass, mid, loud := s.measure(sample)

		now := time.Now()
		if now.Before(next) {
			continue
		}
		next = now.Add(emit)
		f := signal.Frame{
			At: now,
			Values: map[string]float32{
				"bass":     float32(bass),
				"mid":      float32(mid),
				"loudness": float32(loud),
			},
		}
		select {
		case out <- f:
		case <-ctx.Done():
			return nil
		}
	}
}

// measure runs the band filters over one block and returns smoothed,
// normalised band energies.
func (s *Source) measure(samples []float32) (bass, mid, loud float64) {
	var bassSum, midSum, allSum float64
	for _, x := range samples {
		b := s.bassH.step(s.bassL.step(float64(x)))
		m := s.midH.step(s.midL.step(float64(x)))
		bassSum += b * b
		midSum += m * m
		allSum += float64(x) * float64(x)
	}
	if len(samples) == 0 {
		return 0, 0, 0
	}
	n := float64(len(samples))
	bass = s.normalise(s.bassEnv.step(math.Sqrt(bassSum / n)))
	mid = s.normalise(s.midEnv.step(math.Sqrt(midSum / n)))
	loud = s.normalise(s.loudEnv.step(math.Sqrt(allSum / n)))
	return bass, mid, loud
}

// normalise maps an RMS value to 0..1 with a square-root curve, which keeps
// quiet rumble audible while leaving headroom for explosions.
func (s *Source) normalise(rms float64) float64 {
	if rms <= 0 {
		return 0
	}
	v := math.Sqrt(rms) * s.gain
	return clamp01(v)
}

func clamp01(v float64) float64 {
	if v < 0 || v != v {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ---------------------------------------------------------------- DSP -----

// cascade is a chain of biquads. A single 2nd-order section is only 12 dB/oct,
// which leaks a 50 Hz engine note straight into the 160-450 Hz band; three
// sections (36 dB/oct) is enough separation for the bands to stay meaningful.
type cascade [3]filter

func (c *cascade) step(x float64) float64 {
	for i := range c {
		x = c[i].step(x)
	}
	return x
}

func newLowPassCascade(hz, rate float64) cascade {
	f := newLowPass(hz, rate)
	return cascade{f, f, f}
}

func newHighPassCascade(hz, rate float64) cascade {
	f := newHighPass(hz, rate)
	return cascade{f, f, f}
}

// filter is a direct-form-1 biquad.
type filter struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *filter) step(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

// newLowPass builds an RBJ cookbook second-order low-pass at Q = 1/sqrt(2).
func newLowPass(hz, rate float64) filter {
	w0 := 2 * math.Pi * hz / rate
	alpha := math.Sin(w0) / (2 * (1 / math.Sqrt2))
	cw := math.Cos(w0)
	a0 := 1 + alpha
	return filter{
		b0: (1 - cw) / 2 / a0,
		b1: (1 - cw) / a0,
		b2: (1 - cw) / 2 / a0,
		a1: -2 * cw / a0,
		a2: (1 - alpha) / a0,
	}
}

// newHighPass builds an RBJ cookbook second-order high-pass at Q = 1/sqrt(2).
func newHighPass(hz, rate float64) filter {
	w0 := 2 * math.Pi * hz / rate
	alpha := math.Sin(w0) / (2 * (1 / math.Sqrt2))
	cw := math.Cos(w0)
	a0 := 1 + alpha
	return filter{
		b0: (1 + cw) / 2 / a0,
		b1: -(1 + cw) / a0,
		b2: (1 + cw) / 2 / a0,
		a1: -2 * cw / a0,
		a2: (1 - alpha) / a0,
	}
}

// envFollower is a one-pole attack/release smoother: it jumps up quickly and
// falls slowly, which is how a rumble envelope should behave.
type envFollower struct {
	attack, release float64
	y               float64
}

func newEnvFollower(attackMS, releaseMS, rate float64) *envFollower {
	return &envFollower{
		attack:  1 - math.Exp(-1/(attackMS/1000*rate)),
		release: 1 - math.Exp(-1/(releaseMS/1000*rate)),
	}
}

func (e *envFollower) step(x float64) float64 {
	c := e.release
	if x > e.y {
		c = e.attack
	}
	e.y += c * (x - e.y)
	return e.y
}

// detectMonitor finds the default sink's monitor, which is what the user is
// actually hearing.
func detectMonitor() (string, error) {
	if out, err := exec.Command("pactl", "get-default-sink").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name + ".monitor", nil
		}
	}
	out, err := exec.Command("pactl", "list", "short", "sources").Output()
	if err != nil {
		return "", fmt.Errorf("audio: target=auto needs pactl: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 && strings.HasSuffix(fields[1], ".monitor") {
			return fields[1], nil
		}
	}
	return "", errors.New("audio: target=auto found no monitor source")
}
