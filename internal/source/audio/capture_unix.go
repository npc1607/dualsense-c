//go:build !windows

package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func (s *Source) openCapture(ctx context.Context) (io.ReadCloser, func(error) error, error) {
	return s.openCommandCapture(ctx)
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
		fields := strings.Split(line, "\t")
		if len(fields) >= 2 {
			name := strings.TrimSpace(fields[1])
			if strings.HasSuffix(name, ".monitor") {
				return name, nil
			}
		}
	}
	return "", errors.New("audio: target=auto found no monitor source")
}
