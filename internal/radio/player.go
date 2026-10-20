// Package radio provides race engineer / spotter audio broadcasting for the
// DualSense controller's built-in front speaker (audio channels 1 & 2: FL/FR).
//
// DualSense USB audio topology:
//   - Ch1/2 (FL, FR): Built-in Front Speaker / 3.5mm Headphone Jack
//   - Ch3/4 (RL, RR): Haptic Voice Coils (driven by ds5d PCM stream)
//
// Because Ch1/2 and Ch3/4 are distinct channels on the same 4-channel sound
// card, speech broadcasting and haptic feedback run concurrently without conflict.
package radio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultClipsDir points to the repository's assets/radio directory.
const DefaultClipsDir = "assets/radio"

// Player plays race engineer voice lines through the controller's front speaker.
type Player struct {
	sinkName string
	clipsDir string
	logger   *log.Logger

	mu      sync.Mutex
	busy    bool
	current string
}

// Config configures the Player.
type Config struct {
	SinkName string      // PipeWire/PulseAudio sink name (empty = auto-detect)
	ClipsDir string      // Directory containing .wav audio clips
	Logger   *log.Logger // Optional logger
}

// NewPlayer initializes a Player, auto-detecting the DualSense sink if not specified.
func NewPlayer(cfg Config) (*Player, error) {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	if cfg.ClipsDir == "" {
		cfg.ClipsDir = findClipsDir()
	}
	sink := cfg.SinkName
	if sink == "" {
		s, err := FindDualSenseSink()
		if err != nil {
			cfg.Logger.Printf("radio: warning: auto-detect DualSense sink: %v (using default sink)", err)
		} else {
			sink = s
			cfg.Logger.Printf("radio: detected DualSense audio sink: %s", sink)
		}
	}
	return &Player{
		sinkName: sink,
		clipsDir: cfg.ClipsDir,
		logger:   cfg.Logger,
	}, nil
}

// FindDualSenseSink scans PulseAudio / PipeWire for the DualSense 4-channel sink.
func FindDualSenseSink() (string, error) {
	// 1. Try pactl list sinks short
	out, err := exec.Command("pactl", "list", "sinks", "short").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := fields[1]
				lower := strings.ToLower(name)
				if strings.Contains(lower, "dualsense") || strings.Contains(lower, "wireless_controller") {
					return name, nil
				}
			}
		}
	}

	// 2. Try wpctl status
	wpOut, err := exec.Command("wpctl", "status").Output()
	if err == nil {
		lines := strings.Split(string(wpOut), "\n")
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "dualsense") && strings.Contains(lower, "sink") {
				fields := strings.Fields(line)
				for _, f := range fields {
					f = strings.Trim(f, ".*: ")
					if len(f) > 0 && f[0] >= '0' && f[0] <= '9' {
						return f, nil
					}
				}
			}
		}
	}

	return "", errors.New("dualsense audio sink not found")
}

// Play plays an audio file by name or path. If a clip is already playing,
// Play drops or ignores overlapping speech to maintain clear radio discipline.
func (p *Player) Play(ctx context.Context, clipName string) error {
	p.mu.Lock()
	if p.busy {
		p.mu.Unlock()
		return fmt.Errorf("radio busy: currently playing %s", p.current)
	}
	p.busy = true
	p.current = clipName
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.busy = false
		p.current = ""
		p.mu.Unlock()
	}()

	filePath := p.resolvePath(clipName)
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("radio clip %q not found: %w", filePath, err)
	}

	return p.playFile(ctx, filePath)
}

func (p *Player) playFile(ctx context.Context, filePath string) error {
	// Try pw-play first (standard PipeWire)
	if _, err := exec.LookPath("pw-play"); err == nil {
		args := []string{}
		if p.sinkName != "" {
			args = append(args, "--target", p.sinkName)
		}
		args = append(args, filePath)
		cmd := exec.CommandContext(ctx, "pw-play", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try paplay (standard PulseAudio)
	if _, err := exec.LookPath("paplay"); err == nil {
		args := []string{}
		if p.sinkName != "" {
			args = append(args, "--device="+p.sinkName)
		}
		args = append(args, filePath)
		cmd := exec.CommandContext(ctx, "paplay", args...)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try ffplay as fallback
	if _, err := exec.LookPath("ffplay"); err == nil {
		cmd := exec.CommandContext(ctx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", filePath)
		return cmd.Run()
	}

	return errors.New("no audio playback backend available (need pw-play, paplay, or ffplay)")
}

func (p *Player) resolvePath(name string) string {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "./") {
		return name
	}
	if !strings.HasSuffix(name, ".wav") {
		name += ".wav"
	}
	return filepath.Join(p.clipsDir, name)
}

// ListClips returns all available .wav clips found in the clips directory.
func (p *Player) ListClips() ([]string, error) {
	entries, err := os.ReadDir(p.clipsDir)
	if err != nil {
		return nil, err
	}
	var clips []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".wav") {
			clips = append(clips, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	return clips, nil
}

func findClipsDir() string {
	candidates := []string{
		"assets/radio",
		"../assets/radio",
		"../../assets/radio",
		"/usr/local/share/dualsense-c/radio",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return DefaultClipsDir
}
