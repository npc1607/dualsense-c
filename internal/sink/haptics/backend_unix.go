//go:build !windows

package haptics

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

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

func openPlayback(cfg Config, target string, logger *log.Logger) (io.WriteCloser, *exec.Cmd, error) {
	cmdTmpl := cfg.Command
	if cmdTmpl == "" {
		cmdTmpl = defaultCommand
	}
	return openCommandPlayback(cmdTmpl, target, logger)
}

func openPassthroughCapture(src string, logger *log.Logger) (io.ReadCloser, *exec.Cmd, error) {
	cmd := exec.Command("parec", "--device="+src, "--rate="+strconv.Itoa(SampleRate),
		"--channels=2", "--format=float32le", "--latency-msec=20",
		"--process-time-msec=2", "--raw")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("haptics: capture pipe: %w", err)
	}
	cmd.Stderr = &logWriter{log: logger, prefix: "haptics[parec]"}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("haptics: start parec: %w", err)
	}
	return out, cmd, nil
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
