//go:build windows

package audio

import (
	"context"
	"fmt"
	"io"

	"github.com/npc1607/dualsense-c/internal/wasapi"
)

func (s *Source) openCapture(ctx context.Context) (io.ReadCloser, func(error) error, error) {
	if s.customCmd {
		return s.openCommandCapture(ctx)
	}
	stream, err := wasapi.OpenLoopback(s.target, s.rate, s.channels)
	if err != nil {
		return nil, nil, fmt.Errorf("audio: %w", err)
	}
	s.log.Printf("audio: capturing WASAPI loopback %q (%dch %d Hz)", stream.Name(), s.channels, s.rate)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-done:
		}
	}()
	waitFn := func(consumeErr error) error {
		close(done)
		_ = stream.Close()
		if ctx.Err() != nil {
			return nil
		}
		if consumeErr != nil {
			return consumeErr
		}
		return io.EOF
	}
	return stream, waitFn, nil
}

// detectMonitor returns the default Windows render endpoint name for WASAPI
// loopback capture.
func detectMonitor() (string, error) {
	name, err := wasapi.DefaultRenderName()
	if err != nil {
		return "default", nil
	}
	return name, nil
}
