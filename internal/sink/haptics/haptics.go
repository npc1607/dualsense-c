// Package haptics drives the DualSense's voice-coil actuators over the
// controller's USB audio interface instead of the HID rumble bytes.
//
// The HID rumble path can only express "amplitude + one modulation frequency",
// which is why fine surface texture feels coarse on it. The controller is also
// a 4-channel 48 kHz USB sound card, and channels 3/4 are wired straight to the
// left/right voice coils: writing PCM there turns the coils into real
// loudspeakers, so spectrum and transient detail survive.
//
// This package keeps the project cgo-free by streaming PCM through `pw-cat`
// (or any command given in the profile) rather than linking libpipewire.
package haptics

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Sample rate the DualSense audio interface runs at.
const SampleRate = 48000

// Config describes how to reach the coils.
type Config struct {
	// Mode is "synth" (default) or "passthrough".
	//
	//   synth       play a fixed carrier (CarrierHz + harmonics) whose amplitude
	//               comes from the rule output — expressive but one timbre.
	//   passthrough filter the *game audio itself* (LowHz..HighHz) and send it
	//               to the coils, using the rule output as a gain. This is what
	//               gives real HD texture: the coils reproduce the actual
	//               spectrum of gravel / metal / water instead of a buzz.
	Mode string
	// Source is the PipeWire monitor to capture in passthrough mode.
	Source string
	// LowHz/HighHz bound the band sent to the coils in passthrough mode.
	LowHz, HighHz float64
	// Target is a PipeWire sink name, or "auto" to pick the DualSense sink.
	Target string
	// CarrierHz is the fundamental of the synthesised waveform (default 55).
	CarrierHz float64
	// Gain scales the 0..255 rule output into PCM amplitude (default 0.55).
	Gain float64
	// Command is the playback command template. {target}, {rate} and
	// {channels} are substituted. Defaults to pw-cat writing s16le.
	Command string
}

// pacat (PulseAudio compat) honours --device; pw-cat's --target is silently
// ignored by some PipeWire versions, which would play into the wrong sink.
//
// --channel-map is not optional: without it PipeWire picks a "4 channel"
// layout such as front-left,front-center,front-right,rear-center and remixes
// our channels 3/4 into the headphone path, so the voice coils receive
// nothing. Naming the sink's own layout keeps the 4 channels byte-for-byte
// aligned with the hardware, which is what puts our samples on the coils.
const defaultCommand = "pacat --playback --device={target} --rate={rate} --channels={channels} " +
	"--channel-map=front-left,front-right,rear-left,rear-right --format=s16le --latency-msec=10 --process-time-msec=2 --raw"

// Writer mixes a continuous 4-channel stream. Channels 1/2 are kept silent and
// channels 3/4 carry the haptic waveform.
type Writer struct {
	cfg Config
	log *log.Logger

	pcm io.WriteCloser
	cmd *exec.Cmd

	ampL atomic.Uint32 // 0..255
	ampR atomic.Uint32

	// synthesis state, owned by the Run goroutine (or by tests)
	phase            float64
	smoothL, smoothR float64

	// passthrough state
	capCmd *exec.Cmd
	capPCM io.ReadCloser
	pasL   [3]biquad
	pasR   [3]biquad

	mu     sync.Mutex
	closed bool
}

// Open resolves the target and starts the playback process.
func Open(cfg Config, logger *log.Logger) (*Writer, error) {
	if logger == nil {
		logger = log.Default()
	}
	if cfg.CarrierHz <= 0 {
		cfg.CarrierHz = 55
	}
	if cfg.Gain <= 0 {
		cfg.Gain = 0.55
	}
	if cfg.Command == "" {
		cfg.Command = defaultCommand
	}
	if cfg.Mode == "" {
		cfg.Mode = "synth"
	}
	if cfg.Mode != "synth" && cfg.Mode != "passthrough" {
		return nil, fmt.Errorf("haptics: unknown mode %q (synth|passthrough)", cfg.Mode)
	}
	if cfg.Mode == "passthrough" {
		if cfg.HighHz <= 0 {
			cfg.HighHz = 180
		}
		if cfg.LowHz <= 0 {
			cfg.LowHz = 25
		}
		if cfg.LowHz >= cfg.HighHz {
			return nil, fmt.Errorf("haptics: low_hz must be below high_hz")
		}
	}
	target := cfg.Target
	if target == "" || target == "auto" {
		t, err := DetectSink()
		if err != nil {
			return nil, err
		}
		target = t
	}
	repl := strings.NewReplacer("{target}", target, "{rate}", strconv.Itoa(SampleRate),
		"{channels}", "4")
	fields := strings.Fields(repl.Replace(cfg.Command))
	if len(fields) == 0 {
		return nil, fmt.Errorf("haptics: empty command")
	}

	cmd := exec.Command(fields[0], fields[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("haptics: %w", err)
	}
	cmd.Stderr = &logWriter{log: logger, prefix: "haptics[" + fields[0] + "]"}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("haptics: start %s: %w", fields[0], err)
	}
	w := &Writer{cfg: cfg, log: logger, pcm: stdin, cmd: cmd}
	if cfg.Mode == "passthrough" {
		if err := w.openCapture(); err != nil {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			return nil, err
		}
		logger.Printf("haptics: passthrough %q → coils at %d Hz, band %.0f–%.0f Hz",
			target, SampleRate, cfg.LowHz, cfg.HighHz)
	} else {
		logger.Printf("haptics: streaming to %q at %d Hz (carrier %.0f Hz)", target, SampleRate, cfg.CarrierHz)
	}
	return w, nil
}

// Set updates the requested amplitudes (0..255). Safe to call from the sink
// tick; the synthesis loop reads the latest value.
func (w *Writer) Set(left, right uint8) {
	w.ampL.Store(uint32(left))
	w.ampR.Store(uint32(right))
}

// Run synthesises and writes PCM until ctx is cancelled.
//
// Each sample is a small harmonic stack rather than a pure sine: a pure tone
// on a voice coil reads as a test signal, while two or three partials give the
// kind of texture a kerb or a gravel surface actually has.
func (w *Writer) Run(ctx context.Context) error {
	if w.cfg.Mode == "passthrough" {
		defer w.Close()
		return w.runPassthrough(ctx)
	}
	const blockFrames = 96 // 2 ms: smaller blocks cut the output path latency
	buf := make([]byte, blockFrames*4*2)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	start := time.Now()
	written := 0

	for {
		select {
		case <-ctx.Done():
			w.Close()
			return nil
		case <-tick.C:
			// Keep the stream ahead of real time without drifting.
			due := int64(time.Since(start).Seconds() * SampleRate)
			for written+blockFrames <= int(due) {
				w.fill(buf)
				if _, err := w.pcm.Write(buf); err != nil {
					w.Close()
					return fmt.Errorf("haptics: write: %w", err)
				}
				written += blockFrames
			}
		}
	}
}

// Close stops the playback process. Safe to call twice.
func (w *Writer) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	if w.capPCM != nil {
		_ = w.capPCM.Close()
	}
	if w.capCmd != nil {
		_ = w.capCmd.Process.Kill()
		_, _ = w.capCmd.Process.Wait()
	}
	_ = w.pcm.Close()
	if w.cmd != nil {
		_ = w.cmd.Wait()
	}
}

func clampf(v, lo, hi float64) float64 {
	if v < lo || v != v {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DetectSink finds the controller's playback sink via pactl.
func DetectSink() (string, error) {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return "", fmt.Errorf("haptics: cannot list sinks (is pactl installed?): %w", err)
	}
	best := ""
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSpace(fields[1])
		low := strings.ToLower(name)
		if strings.Contains(low, "dualsense") || strings.Contains(low, "054c") {
			best = name
			break
		}
	}
	if best == "" {
		return "", fmt.Errorf("haptics: no DualSense sink found; set haptics.target explicitly (pactl list short sinks)")
	}
	return best, nil
}

// biquad is a direct-form-1 second-order section.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *biquad) step(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

func lowPass(hz, rate float64) biquad {
	w0 := 2 * math.Pi * hz / rate
	alpha := math.Sin(w0) / (2 * (1 / math.Sqrt2))
	cw := math.Cos(w0)
	a0 := 1 + alpha
	return biquad{b0: (1 - cw) / 2 / a0, b1: (1 - cw) / a0, b2: (1 - cw) / 2 / a0,
		a1: -2 * cw / a0, a2: (1 - alpha) / a0}
}

func highPass(hz, rate float64) biquad {
	w0 := 2 * math.Pi * hz / rate
	alpha := math.Sin(w0) / (2 * (1 / math.Sqrt2))
	cw := math.Cos(w0)
	a0 := 1 + alpha
	return biquad{b0: (1 + cw) / 2 / a0, b1: -(1 + cw) / a0, b2: (1 + cw) / 2 / a0,
		a1: -2 * cw / a0, a2: (1 - alpha) / a0}
}

type logWriter struct {
	log    *log.Logger
	prefix string
}

func (l *logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			l.log.Printf("%s: %s", l.prefix, line)
		}
	}
	return len(p), nil
}

// openCapture starts the monitor capture used by passthrough mode.
func (w *Writer) openCapture() error {
	src := w.cfg.Source
	if src == "" || src == "auto" {
		s, err := DetectMonitor()
		if err != nil {
			return err
		}
		src = s
	}
	cmd := exec.Command("parec", "--device="+src, "--rate="+strconv.Itoa(SampleRate),
		"--channels=2", "--format=float32le", "--latency-msec=20",
		"--process-time-msec=2", "--raw")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("haptics: capture pipe: %w", err)
	}
	cmd.Stderr = &logWriter{log: w.log, prefix: "haptics[parec]"}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("haptics: start parec: %w", err)
	}
	w.capCmd, w.capPCM = cmd, out
	for i := range w.pasL {
		w.pasL[i] = highPass(w.cfg.LowHz, SampleRate)
		w.pasR[i] = highPass(w.cfg.LowHz, SampleRate)
	}
	// Two cascaded low-passes per channel: 24 dB/oct keeps the coil band clean.
	w.pasL[2] = lowPass(w.cfg.HighHz, SampleRate)
	w.pasR[2] = lowPass(w.cfg.HighHz, SampleRate)
	return nil
}

// DetectMonitor finds the default sink's monitor source.
func DetectMonitor() (string, error) {
	if out, err := exec.Command("pactl", "get-default-sink").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name + ".monitor", nil
		}
	}
	out, err := exec.Command("pactl", "list", "short", "sources").Output()
	if err != nil {
		return "", fmt.Errorf("haptics: cannot find monitor (pactl missing?): %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) >= 2 && strings.HasSuffix(strings.TrimSpace(f[1]), ".monitor") {
			return strings.TrimSpace(f[1]), nil
		}
	}
	return "", fmt.Errorf("haptics: no monitor source found")
}

// passBlock turns one stereo block of game audio into coil PCM.
//
// The rule output acts as a per-channel gain, so the profile decides *how much*
// of the real texture reaches each coil while the waveform itself stays the
// game's own — that is the difference between a buzz and HD haptics.
func (w *Writer) passBlock(dst []byte, src []float32, frames int) {
	ampL := float64(w.ampL.Load()) / 255
	ampR := float64(w.ampR.Load()) / 255
	w.smoothL += 0.25 * (ampL - w.smoothL)
	w.smoothR += 0.25 * (ampR - w.smoothR)
	for i := 0; i < frames; i++ {
		l := float64(src[i*2])
		r := float64(src[i*2+1])
		for k := range w.pasL {
			l = w.pasL[k].step(l)
			r = w.pasR[k].step(r)
		}
		cl := int16(clampf(l*w.smoothL*w.cfg.Gain, -1, 1) * 32767)
		cr := int16(clampf(r*w.smoothR*w.cfg.Gain, -1, 1) * 32767)
		off := i * 8
		binary.LittleEndian.PutUint16(dst[off:], 0)
		binary.LittleEndian.PutUint16(dst[off+2:], 0)
		binary.LittleEndian.PutUint16(dst[off+4:], uint16(cl))
		binary.LittleEndian.PutUint16(dst[off+6:], uint16(cr))
	}
}

// runPassthrough reads captured audio and writes it to the coils.
func (w *Writer) runPassthrough(ctx context.Context) error {
	const frames = 240 // 5 ms at 48 kHz
	raw := make([]byte, frames*2*4)
	src := make([]float32, frames*2)
	out := make([]byte, frames*8)
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := io.ReadFull(w.capPCM, raw)
		if err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				return nil
			}
			return fmt.Errorf("haptics: capture read: %w", err)
		}
		count := n / 4
		for i := 0; i < count; i++ {
			src[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		got := count / 2
		w.passBlock(out[:got*8], src, got)
		if _, err := w.pcm.Write(out[:got*8]); err != nil {
			return fmt.Errorf("haptics: write: %w", err)
		}
	}
}

// fill renders one block of 4-channel s16le PCM: channels 1/2 stay silent and
// channels 3/4 carry the haptic waveform for the left/right voice coil.
func (w *Writer) fill(buf []byte) {
	ampL := float64(w.ampL.Load()) / 255
	ampR := float64(w.ampR.Load()) / 255
	frames := len(buf) / 8
	for i := 0; i < frames; i++ {
		w.phase += 2 * math.Pi * w.cfg.CarrierHz / SampleRate
		if w.phase > 2*math.Pi {
			w.phase -= 2 * math.Pi
		}
		// One-pole smoothing avoids clicks when a rule switches.
		w.smoothL += 0.25 * (ampL - w.smoothL)
		w.smoothR += 0.25 * (ampR - w.smoothR)

		wave := math.Sin(w.phase) + 0.35*math.Sin(2*w.phase) + 0.12*math.Sin(3*w.phase)
		wave /= 1.47 // keep the harmonic stack inside [-1, 1]

		l := int16(clampf(wave*w.smoothL*w.cfg.Gain, -1, 1) * 32767)
		r := int16(clampf(wave*w.smoothR*w.cfg.Gain, -1, 1) * 32767)

		off := i * 8
		binary.LittleEndian.PutUint16(buf[off:], 0)           // ch1: speaker
		binary.LittleEndian.PutUint16(buf[off+2:], 0)         // ch2: headset
		binary.LittleEndian.PutUint16(buf[off+4:], uint16(l)) // ch3: left coil
		binary.LittleEndian.PutUint16(buf[off+6:], uint16(r)) // ch4: right coil
	}
}
