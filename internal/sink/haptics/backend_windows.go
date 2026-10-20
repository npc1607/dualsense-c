//go:build windows

package haptics

import (
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/npc1607/dualsense-c/internal/wasapi"
)

func openPlayback(cfg Config, target string, logger *log.Logger) (io.WriteCloser, *exec.Cmd, error) {
	if cfg.Command != "" {
		return openCommandPlayback(cfg.Command, target, logger)
	}
	stream, err := wasapi.OpenRender(target, SampleRate)
	if err != nil {
		return nil, nil, fmt.Errorf("haptics: %w", err)
	}
	return stream, nil, nil
}

func openPassthroughCapture(src string, logger *log.Logger) (io.ReadCloser, *exec.Cmd, error) {
	stream, err := wasapi.OpenLoopback(src, SampleRate, 2)
	if err != nil {
		return nil, nil, fmt.Errorf("haptics: %w", err)
	}
	return stream, nil, nil
}

// DetectSink finds the DualSense 4-channel USB audio playback endpoint via WASAPI.
func DetectSink() (string, error) {
	name, err := wasapi.FindRenderDevice("dualsense", "wireless controller", "054c")
	if err != nil {
		return "", fmt.Errorf("haptics: no DualSense audio endpoint found (connect DualSense over USB): %w", err)
	}
	return name, nil
}

// DetectMonitor returns the default Windows render endpoint name for WASAPI loopback.
func DetectMonitor() (string, error) {
	name, err := wasapi.DefaultRenderName()
	if err != nil {
		return "default", nil
	}
	return name, nil
}
