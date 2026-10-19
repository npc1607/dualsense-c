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
const defaultCommand = "pacat --playback --device={target} --rate={rate} --channels={channels} --format=s16le --raw"

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
	logger.Printf("haptics: streaming to %q at %d Hz (carrier %.0f Hz)", target, SampleRate, cfg.CarrierHz)
	return &Writer{cfg: cfg, log: logger, pcm: stdin, cmd: cmd}, nil
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
	const blockFrames = 240 // 5 ms
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
