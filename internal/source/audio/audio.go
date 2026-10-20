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
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/npc1607/dualsense-c/internal/audioai"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("audio", New) }

// defaults, all overridable from the profile's source block.
const (
	// parec (PulseAudio compat) is the reliable one: on this PipeWire version
	// pw-record accepts --target but silently ignores it and records the default
	// node instead, which makes the capture look like permanent silence.
	// Verified on hardware: parec honours --device, pw-record did not.
	defCommand  = "parec --device={target} --rate={rate} --channels={channels} --format={format} --latency-msec=20 --process-time-msec=2 --raw"
	defRate     = 24000
	defChannels = 2
	// parec spells formats as PulseAudio names (float32le), not PipeWire's f32.
	defFormat    = "float32le"
	defEmitHz    = 60
	defBassLow   = 20.0
	defBassHigh  = 110.0
	defMidLow    = 160.0
	defMidHigh   = 450.0
	defGain      = 2.5
	defAttackMS  = 4.0
	defReleaseMS = 90.0
)

// Source turns a system audio monitor into envelope signals.
type Source struct {
	target       string
	customCmd    bool
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

	// Optional in-process classifier (YAMNet via ONNX Runtime).
	clf      *audioai.Classifier
	aiWindow int       // samples per classification window (16000 = 1s)
	aiHop    int       // samples between classifications
	aiRing   []float32 // rolling window
	aiCh     chan map[string]float32
	winCh    chan []float32
	aiSent   int // samples accumulated since the last window was queued
	aiDrop   int // windows dropped because the engine was busy
	aiDebug  bool
	envDebug bool
	envPeak  [3]float32 // bass, mid, loudness peaks since the last report
	rawPeak  float32    // |sample| peak before filtering
	rmsPeak  float32    // loudest block RMS since the last report
	lastBass float32
	lastMid  float32
	lastLoud float32

	// Transient detector state: a fast and a slow tracker of block RMS. The
	// difference spikes on onsets even when the background bed is loud, which
	// an absolute threshold cannot do (game music saturates `loudness`).
	onsetFast    float64
	onsetSlow    float64
	lastOnset    float32
	envPeakOnset float32
	prevLoud     float32
	envLast      time.Time
}

// New builds the source from the profile's `source:` block.
//
// Recognised parameters (all optional):
//
//	target       PipeWire sink or monitor name to record from (required)
//	command      capture command template, {target}/{rate}/{channels}/{format}
//	             (default: parec; pw-record's --target is ignored on some versions)
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

	// YAMNet consumes 16 kHz mono, so the classifier forces that rate; asking to
	// capture at anything else silently breaks it (resampling is not wired up).
	modelPath := getS("model", "")
	classMap := getS("class_map", "")
	var clf *audioai.Classifier
	if modelPath != "" {
		if channels != 1 && channels != 2 {
			return nil, errors.New("audio source: model requires --channels 1 or 2")
		}
		if rate != 16000 {
			return nil, fmt.Errorf("audio source: model requires rate 16000 (YAMNet input); got %d", rate)
		}
		if classMap == "" {
			classMap = "models/yamnet_class_map.csv"
		}
		engine, err := audioai.Open(modelPath, classMap, getI("threads", 2))
		if err != nil {
			return nil, fmt.Errorf("audio source: %w", err)
		}
		clf = engine
		if deps.Logger != nil {
			deps.Logger.Printf("audio: AI classifier loaded (%s)", modelPath)
		}
	}
	format := getS("format", defFormat)

	customCmd := getS("command", "") != ""
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
		target:    target,
		customCmd: customCmd,
		cmd:       cmd,
		rate:      rate,
		channels:  channels,
		emitHz:    getI("rate_hz", defEmitHz),
		gain:      getF("gain", defGain),
		bassL:     newHighPassCascade(bassLo, float64(rate)),
		bassH:     newLowPassCascade(bassHi, float64(rate)),
		midL:      newHighPassCascade(midLo, float64(rate)),
		midH:      newLowPassCascade(midHi, float64(rate)),
		log:       logger,
	}
	if s.emitHz <= 0 {
		s.emitHz = defEmitHz
	}
	if clf != nil {
		s.clf = clf
		s.aiWindow = 16000
		// YAMNet needs ~1 s of audio, so the window cannot shrink; the hop can.
		// 100 ms gives 10 updates/s at ~5 ms inference cost each.
		hopMS := getF("classify_ms", 250)
		if hopMS < 100 {
			hopMS = 100
		}
		s.aiHop = int(float64(rate) * hopMS / 1000)
		if s.aiHop < 1 {
			s.aiHop = 1
		}
		s.aiRing = make([]float32, 0, s.aiWindow)
		s.aiDebug = false
		if v, ok := cfg.Params["ai_debug"]; ok {
			if b, ok := v.(bool); ok {
				s.aiDebug = b
			}
		}
		if v, ok := cfg.Params["env_debug"]; ok {
			if b, ok := v.(bool); ok {
				s.envDebug = b
			}
		}
		s.aiCh = make(chan map[string]float32, 4)
		s.winCh = make(chan []float32, 2)
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

// Run opens the capture stream and processes its PCM stream until ctx ends.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	rc, waitFn, err := s.openCapture(ctx)
	if err != nil {
		return err
	}
	if s.clf != nil {
		go s.classifyLoop(ctx)
	}
	err = s.consume(ctx, rc, out)
	return waitFn(err)
}

func (s *Source) openCommandCapture(ctx context.Context) (io.ReadCloser, func(error) error, error) {
	cmd := exec.CommandContext(ctx, s.cmd[0], s.cmd[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("audio: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("audio: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("audio: start %s: %w", s.cmd[0], err)
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

	waitFn := func(consumeErr error) error {
		_ = cmd.Process.Kill()
		werr := cmd.Wait()
		if ctx.Err() != nil {
			return nil
		}
		if consumeErr != nil {
			return consumeErr
		}
		if werr != nil {
			return fmt.Errorf("audio: capture command exited: %w", werr)
		}
		return io.EOF
	}
	return stdout, waitFn, nil
}

// idleAfter is how long the capture may stay silent before we start emitting
// decaying frames ourselves. PipeWire suspends an idle sink and stops pushing
// audio, so a blocking read would stall the source forever and leave the last
// loud value frozen downstream.
const idleAfter = 80 * time.Millisecond

// classifyLoop runs the model off the capture goroutine: inference takes a few
// milliseconds and must not stall the audio reader.
func (s *Source) classifyLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			if s.clf != nil {
				_ = s.clf.Close()
			}
			return
		case win := <-s.winCh:
			res, err := s.clf.Signals(win)
			if err != nil {
				s.log.Printf("audio: classify: %v", err)
				continue
			}
			if s.aiDebug {
				scores, err := s.clf.RawScores(win)
				if err == nil {
					top := s.clf.TopN(scores, 3)
					parts := make([]string, 0, len(top))
					for _, t := range top {
						parts = append(parts, fmt.Sprintf("%s=%.2f", t.Name, t.Score))
					}
					s.log.Printf("audio: ai %s", strings.Join(parts, " "))
				}
			}
			// Keep only the newest result; the rules engine reads at 60 Hz while
			// inference runs at ~2 Hz.
			for {
				select {
				case <-s.aiCh:
					continue
				default:
				}
				break
			}
			select {
			case s.aiCh <- res:
			default:
			}
		}
	}
}

// latestAI returns the most recent classification, if any.
func (s *Source) latestAI() map[string]float32 {
	if s.clf == nil {
		return nil
	}
	var out map[string]float32
	for {
		select {
		case v := <-s.aiCh:
			out = v
			continue
		default:
		}
		return out
	}
}

// feedAI appends a block to the rolling window and queues a classification when
// a full window is available.
func (s *Source) feedAI(block []float32) {
	if s.clf == nil {
		return
	}
	s.aiRing = append(s.aiRing, block...)
	if len(s.aiRing) < s.aiWindow {
		return
	}
	s.aiSent += len(block)
	if s.aiSent < s.aiHop {
		return
	}
	s.aiSent = 0
	win := make([]float32, s.aiWindow)
	copy(win, s.aiRing[len(s.aiRing)-s.aiWindow:])
	select {
	case s.winCh <- win:
	default:
		s.aiDrop++
	}
	if len(s.aiRing) > 2*s.aiWindow {
		s.aiRing = append(s.aiRing[:0], s.aiRing[len(s.aiRing)-s.aiWindow:]...)
	}
}

// consume decodes f32le PCM, filters it into bands and emits envelope frames.
//
// Reads use a deadline slightly longer than one block so an idle sink (PipeWire
// suspends it and stops pushing audio) cannot stall the source: on timeout we
// feed the envelope a silent block so rumble fades out instead of freezing.
func (s *Source) consume(ctx context.Context, r io.Reader, out chan<- signal.Frame) error {
	const blockFrames = 256 // ~43 ms at 24 kHz: must fit inside idleAfter
	frameBytes := 4 * s.channels
	raw := make([]byte, blockFrames*frameBytes)
	var pending []byte
	sample := make([]float32, 0, blockFrames)
	emit := time.Second / time.Duration(s.emitHz)
	next := time.Now()
	deadline := time.Now().Add(idleAfter)

	flush := func() bool { // returns false when ctx is done
		now := time.Now()
		if now.Before(next) {
			return true
		}
		next = now.Add(emit)
		vals := map[string]float32{
			"bass":     s.lastBass,
			"mid":      s.lastMid,
			"loudness": s.lastLoud,
			"onset":    s.lastOnset,
		}
		if s.envDebug {
			if s.prevLoud < 0.08 && vals["loudness"] >= 0.08 {
				s.log.Printf("audio: onset loudness=%.3f at=%s", vals["loudness"], now.Format("15:04:05.000"))
			}
			s.prevLoud = vals["loudness"]
			if vals["bass"] > s.envPeak[0] {
				s.envPeak[0] = vals["bass"]
			}
			if vals["mid"] > s.envPeak[1] {
				s.envPeak[1] = vals["mid"]
			}
			if vals["loudness"] > s.envPeak[2] {
				s.envPeak[2] = vals["loudness"]
			}
			if vals["onset"] > s.envPeakOnset {
				s.envPeakOnset = vals["onset"]
			}
			if s.envLast.IsZero() {
				s.envLast = now
			} else if now.Sub(s.envLast) >= time.Second {
				s.log.Printf("audio: env peak raw=%.3f rms=%.4f bass=%.3f mid=%.3f loudness=%.3f onset=%.3f",
					s.rawPeak, s.rmsPeak, s.envPeak[0], s.envPeak[1], s.envPeak[2], s.envPeakOnset)
				s.rawPeak = 0
				s.rmsPeak = 0
				s.envPeak = [3]float32{}
				s.envPeakOnset = 0
				s.envLast = now
			}
		}
		for k, v := range s.latestAI() {
			vals[k] = v
		}
		select {
		case out <- signal.Frame{At: now, Values: vals}:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for {
		if ctx.Err() != nil {
			return nil
		}
		if f, ok := r.(interface{ SetReadDeadline(time.Time) error }); ok {
			_ = f.SetReadDeadline(deadline)
		}
		n, err := r.Read(raw)
		if n > 0 {
			deadline = time.Now().Add(idleAfter)
			pending = append(pending, raw[:n]...)
			frames := len(pending) / frameBytes
			if frames > 0 {
				sample = sample[:0]
				for i := 0; i < frames; i++ {
					var sum float32
					for c := 0; c < s.channels; c++ {
						off := i*frameBytes + c*4
						sum += math.Float32frombits(binary.LittleEndian.Uint32(pending[off:]))
					}
					sample = append(sample, sum/float32(s.channels))
				}
				pending = pending[frames*frameBytes:]
				s.feedAI(sample)
				s.measure(sample)
				if !flush() {
					return nil
				}
			}
		}
		if err != nil {
			if isTimeout(err) {
				s.measure(nil) // decay the envelopes
				if !flush() {
					return nil
				}
				deadline = time.Now().Add(idleAfter)
				continue
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return fmt.Errorf("audio: read: %w", err)
		}
	}
}

// measure runs the band filters over one block and returns smoothed,
// normalised band energies.
func (s *Source) measure(samples []float32) (bass, mid, loud float64) {
	var bassSum, midSum, allSum float64
	for _, x := range samples {
		if a := math.Abs(float64(x)); a > float64(s.rawPeak) {
			s.rawPeak = float32(a)
		}
		b := s.bassH.step(s.bassL.step(float64(x)))
		m := s.midH.step(s.midL.step(float64(x)))
		bassSum += b * b
		midSum += m * m
		allSum += float64(x) * float64(x)
	}
	n := float64(len(samples))
	if n > 0 {
		if rms := math.Sqrt(allSum / n); rms > float64(s.rmsPeak) {
			s.rmsPeak = float32(rms)
		}
	}
	if n == 0 {
		// Still step the envelopes down so silence actually decays.
		n = 1
	}
	steps := len(samples)
	bass = s.normalise(s.bassEnv.stepN(math.Sqrt(bassSum/n), steps))
	mid = s.normalise(s.midEnv.stepN(math.Sqrt(midSum/n), steps))
	loud = s.normalise(s.loudEnv.stepN(math.Sqrt(allSum/n), steps))
	// Emit the *normalised* values: using env.y directly meant `gain` had no
	// effect on the signals at all, which kept game audio stuck near the
	// thresholds no matter how the profile was tuned.
	s.lastBass, s.lastMid, s.lastLoud = float32(bass), float32(mid), float32(loud)

	// Onset = how far the fast tracker is above the slow one, relative to the
	// slow level so a loud bed does not mask it.
	rmsNow := math.Sqrt(allSum / n)
	if rmsNow > s.onsetFast {
		s.onsetFast += 0.6 * (rmsNow - s.onsetFast) // jump up quickly
	} else {
		s.onsetFast += 0.15 * (rmsNow - s.onsetFast)
	}
	s.onsetSlow += 0.05 * (rmsNow - s.onsetSlow) // ~300ms bed tracker
	jump := s.onsetFast - s.onsetSlow
	s.lastOnset = float32(clamp01(jump / (s.onsetSlow + 0.002)))
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

// stepN applies the one-pole law over n samples at once.
//
// The coefficients are per-sample, and measure() runs once per block, so
// stepping only once per block would stretch a 90 ms release into ~23 s.
func (e *envFollower) stepN(x float64, n int) float64 {
	if n < 1 {
		n = 1
	}
	c := e.release
	if x > e.y {
		c = e.attack
	}
	k := math.Pow(1-c, float64(n))
	e.y = x + (e.y-x)*k
	return e.y
}

// isTimeout reports whether err is a read deadline expiry.
func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	var t timeout
	if errors.As(err, &t) {
		return t.Timeout()
	}
	return os.IsTimeout(err)
}
