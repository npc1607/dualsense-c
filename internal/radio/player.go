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
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultClipsDir points to the repository's assets/radio directory.
const DefaultClipsDir = "assets/radio"

// Player plays race engineer voice lines through the controller's front speaker.
type Player struct {
	sinkName string
	clipsDir string
	logger   *log.Logger
	style    AcousticStyle

	mu            sync.Mutex
	busy          bool
	current       string
	currentPri    int
	cancelCurrent context.CancelFunc
	currentGen    uint64
	currentDone   chan struct{}
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

// SetStyle configures the active DSP acoustic style filter.
func (p *Player) SetStyle(style AcousticStyle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.style = style
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

// IsBusy reports whether the player is currently executing audio playback.
func (p *Player) IsBusy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.busy
}

// Play plays an audio file by name or path at normal priority (0).
func (p *Player) Play(ctx context.Context, clipName string) error {
	return p.PlayWithPriority(ctx, clipName, 0)
}

func (p *Player) resolveFallbackBeep() string {
	beepPath := filepath.Join(p.clipsDir, "beep_in.wav")
	if _, err := os.Stat(beepPath); err == nil {
		return beepPath
	}
	fallbackPath := filepath.Join(os.TempDir(), "ds5_fallback_beep.wav")
	if _, err := os.Stat(fallbackPath); err == nil {
		return fallbackPath
	}

	sampleRate := 48000
	numSamples := int(0.1 * float64(sampleRate))
	samples := make([]float32, numSamples)
	ramp := int(0.005 * float64(sampleRate))
	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		env := 1.0
		if i < ramp {
			env = float64(i) / float64(ramp)
		} else if i > numSamples-ramp {
			env = float64(numSamples-i) / float64(ramp)
		}
		samples[i] = float32(0.5 * env * math.Sin(2.0*math.Pi*800.0*t))
	}
	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", fallbackPath, os.Getpid(), time.Now().UnixNano())
	f, err := os.Create(tmpPath)
	if err == nil {
		_ = WriteWAV(f, samples, sampleRate)
		_ = f.Close()
		_ = os.Rename(tmpPath, fallbackPath)
	}
	return fallbackPath
}

// PlayWithPriority plays an audio file with preemption support. If an audio
// clip is already playing, incoming speech can preempt it if its priority
// is at least 30 levels higher (e.g. Crash 100 preempts Pace 30).
func (p *Player) PlayWithPriority(ctx context.Context, clipName string, priority int) error {
	// 1. Pre-validate file path and apply graceful fallback beep if missing
	filePath := p.resolvePath(clipName)
	if _, err := os.Stat(filePath); err != nil {
		if p.logger != nil {
			p.logger.Printf("radio/player: clip %q not found, using fallback beep", clipName)
		}
		filePath = p.resolveFallbackBeep()
	}

	// 2. Apply DSP acoustic styling if configured
	finalPath := filePath
	p.mu.Lock()
	style := p.style
	p.mu.Unlock()

	if style != "" && style != StyleStudio {
		styledName := fmt.Sprintf("ds5_styled_%s_%s", style, filepath.Base(filePath))
		styledPath := filepath.Join(os.TempDir(), styledName)
		if fi, err := os.Stat(styledPath); err != nil || fi.Size() == 0 {
			if err := ProcessWAV(filePath, styledPath, style); err == nil {
				finalPath = styledPath
			}
		} else {
			finalPath = styledPath
		}
	}

	// 3. Preemption handling: wait for previous player process to fully terminate
	for {
		p.mu.Lock()
		if !p.busy {
			break
		}
		if priority < p.currentPri+30 || p.cancelCurrent == nil {
			cur := p.current
			curPri := p.currentPri
			p.mu.Unlock()
			return fmt.Errorf("radio busy: currently playing %s (pri %d)", cur, curPri)
		}

		if p.logger != nil {
			p.logger.Printf("radio/player: preempting %q (pri %d) with %q (pri %d)",
				p.current, p.currentPri, clipName, priority)
		}
		oldDone := p.currentDone
		p.cancelCurrent()
		p.cancelCurrent = nil
		if oldDone == nil {
			p.busy = false
			break
		}
		p.mu.Unlock()

		select {
		case <-oldDone:
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	p.currentGen++
	thisGen := p.currentGen
	playCtx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})

	p.busy = true
	p.current = clipName
	p.currentPri = priority
	p.cancelCurrent = cancel
	p.currentDone = doneCh
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		if p.currentGen == thisGen {
			p.busy = false
			p.current = ""
			p.currentPri = 0
			p.cancelCurrent = nil
			p.currentDone = nil
		}
		p.mu.Unlock()
		cancel()
		close(doneCh)
	}()

	return p.playFile(playCtx, finalPath)
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
		if ctx.Err() != nil {
			return ctx.Err()
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
		if ctx.Err() != nil {
			return ctx.Err()
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

// Stop interrupts any running playback immediately.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelCurrent != nil {
		p.cancelCurrent()
	}
}
