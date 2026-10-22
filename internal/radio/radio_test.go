package radio

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
)

func TestPlayerListClips(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "clip1.wav"), []byte("RIFFtest"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "clip2.WAV"), []byte("RIFFtest"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "ignore.txt"), []byte("text"), 0644)

	p, err := NewPlayer(Config{
		ClipsDir: tmpDir,
		SinkName: "test_sink",
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}

	clips, err := p.ListClips()
	if err != nil {
		t.Fatalf("ListClips: %v", err)
	}
	if len(clips) != 2 {
		t.Errorf("expected 2 clips, got %d: %v", len(clips), clips)
	}
}

func TestDefaultTriggers(t *testing.T) {
	triggers := DefaultTriggers()
	if len(triggers) == 0 {
		t.Fatal("expected default triggers")
	}

	// 1. Test tire slip
	var slipTrig *Trigger
	for i := range triggers {
		if triggers[i].Name == "tire_slip" {
			slipTrig = &triggers[i]
			break
		}
	}
	if slipTrig == nil {
		t.Fatal("missing tire_slip trigger")
	}

	// Low speed or no slip -> should not fire
	valsNoSlip := map[string]float32{"speed": 50, "slip": 0.2}
	st := &TriggerState{}
	if slipTrig.Evaluate(valsNoSlip, 16*time.Millisecond, st) {
		t.Error("tire_slip fired unexpectedly on clean grip")
	}

	// High speed and high slip -> should fire
	valsSlip := map[string]float32{"speed": 50, "slip": 1.4}
	if !slipTrig.Evaluate(valsSlip, 16*time.Millisecond, st) {
		t.Error("tire_slip should have fired on high slip")
	}

	// 2. Test redline duration persistence
	var redlineTrig *Trigger
	for i := range triggers {
		if triggers[i].Name == "engine_redline" {
			redlineTrig = &triggers[i]
			break
		}
	}
	if redlineTrig == nil {
		t.Fatal("missing engine_redline trigger")
	}

	stRedline := &TriggerState{}
	valsHighRPM := map[string]float32{"custom0": 7500}

	// 500ms -> not long enough
	if redlineTrig.Evaluate(valsHighRPM, 500*time.Millisecond, stRedline) {
		t.Error("redline fired too quickly (< 1.2s)")
	}

	// another 800ms -> total 1300ms -> should fire!
	if !redlineTrig.Evaluate(valsHighRPM, 800*time.Millisecond, stRedline) {
		t.Error("redline should have fired after 1.3s total duration")
	}
}

func TestResolvePath(t *testing.T) {
	p := &Player{clipsDir: "/var/clips"}
	if got := p.resolvePath("test"); got != "/var/clips/test.wav" {
		t.Errorf("expected /var/clips/test.wav, got %s", got)
	}
	if got := p.resolvePath("test.wav"); got != "/var/clips/test.wav" {
		t.Errorf("expected /var/clips/test.wav, got %s", got)
	}
	if got := p.resolvePath("/tmp/custom.wav"); got != "/tmp/custom.wav" {
		t.Errorf("expected /tmp/custom.wav, got %s", got)
	}
}

func TestTelemetryWindow_SlidingAndMetrics(t *testing.T) {
	w := NewTelemetryWindow(5 * time.Second)
	t0 := time.Now()

	// 1. Push frames spaced 1 second apart: t0 to t0+6s (7 frames)
	for i := 0; i <= 6; i++ {
		w.Push(signal.Frame{
			Values: map[string]float32{
				"speed": float32(50 + i*10),
				"rpm":   float32(6000 + i*200),
			},
			At: t0.Add(time.Duration(i) * time.Second),
		})
	}

	// Because retention is 5s, frames older than (t0+6s - 5s = t0+1s) should be evicted.
	// Frames retained: t0+1s, t0+2s, t0+3s, t0+4s, t0+5s, t0+6s (6 frames)
	if count := w.Count(); count != 6 {
		t.Errorf("expected 6 frames in window, got %d", count)
	}

	// Check LatestVal
	if val := w.LatestVal("speed"); val != 110.0 {
		t.Errorf("expected latest speed 110.0, got %f", val)
	}

	// Check DurationAbove: rpm has been >= 7000 from t0+5s (7000) to t0+6s (7200) -> 1 second
	dur := w.DurationAbove("rpm", 7000)
	if dur != 1*time.Second {
		t.Errorf("expected DurationAbove 7000 to be 1s, got %v", dur)
	}

	// Check Derivative: speed from t0+5s (100) to t0+6s (110) with dt=1s -> 10.0 / s
	deriv := w.Derivative("speed")
	if deriv < 9.9 || deriv > 10.1 {
		t.Errorf("expected derivative ~10.0, got %f", deriv)
	}

	// Check Average: speed over last 2 seconds (t0+5s: 100, t0+6s: 110) -> 105.0
	avg := w.Average("speed", 2*time.Second)
	// cutoff is t0+4s: t0+4s (90), t0+5s (100), t0+6s (110)
	if avg < 90 || avg > 115 {
		t.Errorf("expected avg around 100-105, got %f", avg)
	}

	// Test Clear
	w.Clear()
	if w.Count() != 0 {
		t.Errorf("expected 0 frames after clear, got %d", w.Count())
	}
}

func TestRuleClassifier_Priorities(t *testing.T) {
	c := NewRuleClassifier()
	defer c.Close()
	ctx := context.Background()

	// 1. Crash telemetry
	wCrash := NewTelemetryWindow(5 * time.Second)
	t0 := time.Now()
	wCrash.Push(signal.Frame{
		Values: map[string]float32{"speed": 60, "accel_g": 1.2},
		At:     t0,
	})
	intent, err := c.Classify(ctx, wCrash)
	if err != nil {
		t.Fatalf("classify crash: %v", err)
	}
	if intent == nil || intent.ClipName != "heavy_crash" || intent.Priority != 100 {
		t.Fatalf("expected heavy_crash with pri 100, got %+v", intent)
	}

	// 2. Slip + Crash simultaneously -> heavy_crash wins (Priority 100 > 50)
	wBoth := NewTelemetryWindow(5 * time.Second)
	wBoth.Push(signal.Frame{
		Values: map[string]float32{
			"speed":   60,
			"accel_g": 1.5,
			"slip":    2.0,
		},
		At: t0,
	})
	intentBoth, err := c.Classify(ctx, wBoth)
	if err != nil {
		t.Fatalf("classify both: %v", err)
	}
	if intentBoth == nil || intentBoth.ClipName != "heavy_crash" {
		t.Fatalf("expected heavy_crash to win over slip, got %+v", intentBoth)
	}

	// 3. Redline sustained for 1.3s -> engine_redline
	wRedline := NewTelemetryWindow(5 * time.Second)
	wRedline.Push(signal.Frame{
		Values: map[string]float32{"rpm": 7500},
		At:     t0,
	})
	wRedline.Push(signal.Frame{
		Values: map[string]float32{"rpm": 7600},
		At:     t0.Add(500 * time.Millisecond),
	})
	// Only 500ms elapsed -> should not fire
	intentShort, _ := c.Classify(ctx, wRedline)
	if intentShort != nil {
		t.Fatalf("redline fired prematurely at 500ms: %+v", intentShort)
	}
	// 1.3s elapsed -> should fire
	wRedline.Push(signal.Frame{
		Values: map[string]float32{"rpm": 7650},
		At:     t0.Add(1300 * time.Millisecond),
	})
	intentRedline, err := c.Classify(ctx, wRedline)
	if err != nil {
		t.Fatalf("classify redline: %v", err)
	}
	if intentRedline == nil || intentRedline.ClipName != "engine_redline" || intentRedline.Priority != 70 {
		t.Fatalf("expected engine_redline, got %+v", intentRedline)
	}

	// 4. Fastest pace sustained for 3.6s -> fastest_pace
	wPace := NewTelemetryWindow(5 * time.Second)
	wPace.Push(signal.Frame{
		Values: map[string]float32{"speed": 125, "slip": 0.1},
		At:     t0,
	})
	wPace.Push(signal.Frame{
		Values: map[string]float32{"speed": 130, "slip": 0.15},
		At:     t0.Add(3600 * time.Millisecond),
	})
	intentPace, err := c.Classify(ctx, wPace)
	if err != nil {
		t.Fatalf("classify pace: %v", err)
	}
	if intentPace == nil || intentPace.ClipName != "fastest_pace" || intentPace.Priority != 30 {
		t.Fatalf("expected fastest_pace, got %+v", intentPace)
	}

	// 5. Strategy pit / box call -> f1_box
	wBox := NewTelemetryWindow(5 * time.Second)
	wBox.Push(signal.Frame{
		Values: map[string]float32{"box": 1.0},
		At:     t0,
	})
	intentBox, err := c.Classify(ctx, wBox)
	if err != nil {
		t.Fatalf("classify box: %v", err)
	}
	if intentBox == nil || intentBox.ClipName != "f1_box" || intentBox.Priority != 40 {
		t.Fatalf("expected f1_box, got %+v", intentBox)
	}
}

func TestArbitrator_Discipline(t *testing.T) {
	arb := NewArbitrator(1500 * time.Millisecond) // 1.5s spacing
	t0 := time.Now()

	pace := &RadioIntent{
		ID:       "fastest_pace",
		ClipName: "fastest_pace",
		Priority: 30,
		Cooldown: 45 * time.Second,
	}

	slip := &RadioIntent{
		ID:       "tire_slip",
		ClipName: "tire_slip",
		Priority: 50,
		Cooldown: 10 * time.Second,
	}

	crash := &RadioIntent{
		ID:       "heavy_crash",
		ClipName: "heavy_crash",
		Priority: 100,
		Cooldown: 15 * time.Second,
	}

	// 1. First broadcast permitted
	if !arb.ShouldPlay(pace, t0) {
		t.Fatal("expected pace to be permitted initially")
	}
	arb.MarkPlaying(pace, t0, 3*time.Second) // Plays for 3s (until t0+3s)

	// 2. Cooldown rejection: same intent within cooldown -> rejected
	if arb.ShouldPlay(pace, t0.Add(10*time.Second)) {
		t.Fatal("expected pace to be rejected within its 45s cooldown")
	}

	// 3. Busy rejection: slip (pri 50) tries to play while pace (pri 30) is active at t0+1s
	// 50 < 30 + 30 (60) -> cannot preempt!
	if arb.ShouldPlay(slip, t0.Add(1*time.Second)) {
		t.Fatal("expected slip to be rejected while pace is busy")
	}

	// 4. Preemption allowed: crash (pri 100) arrives at t0+1s
	// 100 >= 30 + 30 (60) -> PREEMPTION!
	if !arb.ShouldPlay(crash, t0.Add(1*time.Second)) {
		t.Fatal("expected crash (pri 100) to preempt pace (pri 30)")
	}
	arb.MarkPlaying(crash, t0.Add(1*time.Second), 3*time.Second) // Crash now busy until t0+4s

	// 5. Spacing rejection: at t0+4.5s (after crash finishes at t0+4s), slip arrives
	// But minSpacing is 1.5s and crash was played at t0+1s
	// Let's reset arbitrator to test clean spacing without preemption:
	arb.Reset()
	arb.MarkPlaying(slip, t0, 500*time.Millisecond) // slip finishes at t0+0.5s

	// At t0+1.0s, pace arrives. Not busy (0.5s passed), but spacing is 1.5s (only 1.0s elapsed):
	if arb.ShouldPlay(pace, t0.Add(1000*time.Millisecond)) {
		t.Fatal("expected pace to be rejected by minSpacing (1.0s < 1.5s)")
	}

	// At t0+1.6s, spacing has elapsed (1.6s >= 1.5s):
	if !arb.ShouldPlay(pace, t0.Add(1600*time.Millisecond)) {
		t.Fatal("expected pace to be allowed after minSpacing elapsed")
	}
}

func TestAcousticPipeline_Styles(t *testing.T) {
	// 1. Verify ParseStyle
	if ParseStyle("radio") != StyleRadio ||
		ParseStyle("STUDIO") != StyleStudio ||
		ParseStyle("mech") != StyleMech ||
		ParseStyle("whisper") != StyleWhisper ||
		ParseStyle("invalid") != StyleRadio {
		t.Errorf("ParseStyle failed mapping styles")
	}

	// 2. Generate test sine wave at 440 Hz
	sampleRate := 48000
	numSamples := 480 // 10ms
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = float32(0.8 * math.Sin(2.0*math.Pi*440.0*float64(i)/float64(sampleRate)))
	}

	styles := []AcousticStyle{StyleRadio, StyleStudio, StyleMech, StyleWhisper}
	for _, style := range styles {
		out := RenderPCM(samples, sampleRate, style)
		if len(out) != len(samples) {
			t.Fatalf("style %s: expected %d samples, got %d", style, len(samples), len(out))
		}

		hasNonZero := false
		for i, s := range out {
			if s > 1.0 || s < -1.0 {
				t.Fatalf("style %s: sample %d out of bounds [-1, 1]: %f", style, i, s)
			}
			if s != 0 {
				hasNonZero = true
			}
		}
		if !hasNonZero {
			t.Fatalf("style %s: rendered output is completely silent", style)
		}
	}

	// Test empty slice
	if out := RenderPCM(nil, 48000, StyleRadio); out != nil {
		t.Errorf("expected nil for empty slice, got %v", out)
	}
}

func TestPlayer_PreemptionAndStop(t *testing.T) {
	p := &Player{
		logger: log.New(io.Discard, "", 0),
	}

	// Initial idle state
	if p.IsBusy() {
		t.Fatal("expected player to be idle initially")
	}

	cancelled := false
	p.mu.Lock()
	p.busy = true
	p.current = "lap_time"
	p.currentPri = 30
	p.cancelCurrent = func() {
		cancelled = true
	}
	p.mu.Unlock()

	if !p.IsBusy() {
		t.Fatal("expected player to be busy")
	}

	// 1. Same or slight higher priority (50 < 30 + 30) should be rejected
	err := p.PlayWithPriority(context.Background(), "mild_alert", 50)
	if err == nil {
		t.Fatal("expected PlayWithPriority to fail when busy with insufficient priority delta")
	}
	if cancelled {
		t.Fatal("cancelCurrent should not be called when preemption rejected")
	}

	// 2. High priority (100 >= 30 + 30) should trigger cancellation
	_ = p.PlayWithPriority(context.Background(), "nonexistent_crash", 100)
	if !cancelled {
		t.Fatal("expected cancelCurrent to be invoked for high priority preemption (100 vs 30)")
	}

	// 3. Test Stop()
	stopCalled := false
	p.mu.Lock()
	p.cancelCurrent = func() {
		stopCalled = true
	}
	p.mu.Unlock()

	p.Stop()
	if !stopCalled {
		t.Fatal("expected Stop() to invoke cancelCurrent")
	}
}

func TestPersona_TelemetryPromptFormatting(t *testing.T) {
	w := NewTelemetryWindow(5 * time.Second)
	t0 := time.Now()
	w.Push(signal.Frame{
		Values: map[string]float32{
			"speed":   120.0,
			"rpm":     7300.0,
			"slip":    1.4,
			"hp":      25.0,
			"stamina": 0.1,
			"hazard":  0.85,
			"box":     1.0,
		},
		At: t0,
	})

	prompt := FormatTelemetryPrompt(PersonaF1Engineer, w)

	// Check ChatML tags
	if !strings.Contains(prompt, "<|im_start|>system") ||
		!strings.Contains(prompt, "<|im_start|>user") ||
		!strings.Contains(prompt, "<|im_start|>assistant") {
		t.Fatalf("prompt missing ChatML structure:\n%s", prompt)
	}

	// Check metric values
	if !strings.Contains(prompt, "speed=120.0") ||
		!strings.Contains(prompt, "rpm=7300") ||
		!strings.Contains(prompt, "hazard=0.85") {
		t.Fatalf("prompt missing expected telemetry metrics:\n%s", prompt)
	}
}

func TestLLMClassifier_OfflineAndFallback(t *testing.T) {
	offlineEng := NewOfflineLLMEngine()
	defer offlineEng.Close()

	logger := log.New(io.Discard, "", 0)
	classifier := NewLLMClassifier(offlineEng, PersonaF1Engineer, logger)
	defer classifier.Close()

	ctx := context.Background()
	w := NewTelemetryWindow(5 * time.Second)
	t0 := time.Now()

	// 1. LLM mock response with structured JSON
	offlineEng.SetMockResponse(`{"category": "warning", "priority": 95, "clip": "custom_alarm", "text": "Extreme hazard!", "cooldown_sec": 8}`)
	w.Push(signal.Frame{
		Values: map[string]float32{"speed": 50},
		At:     t0,
	})

	intent, err := classifier.Classify(ctx, w)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if intent == nil || intent.ClipName != "custom_alarm" || intent.Priority != 95 || intent.Cooldown != 8*time.Second {
		t.Fatalf("unexpected intent from mock LLM: %+v", intent)
	}

	// 2. LLM returns markdown fenced JSON
	offlineEng.SetMockResponse("```json\n{\"category\": \"strategy\", \"priority\": 45, \"clip\": \"box_now\", \"text\": \"Box this lap\", \"cooldown_sec\": 30}\n```")
	intentBox, err := classifier.Classify(ctx, w)
	if err != nil {
		t.Fatalf("classify markdown error: %v", err)
	}
	if intentBox == nil || intentBox.ClipName != "box_now" || intentBox.Priority != 45 {
		t.Fatalf("unexpected intent from markdown LLM: %+v", intentBox)
	}

	// 3. LLM returns empty JSON -> active silence (no fallback)
	offlineEng.SetMockResponse("{}")
	intentSilence, err := classifier.Classify(ctx, w)
	if err != nil || intentSilence != nil {
		t.Fatalf("expected active silence (nil, nil) for {}, got %+v, %v", intentSilence, err)
	}

	// 4. LLM returns malformed non-JSON -> triggers fallback RuleClassifier
	offlineEng.SetMockResponse("malformed non-json response")
	wCrash := NewTelemetryWindow(5 * time.Second)
	wCrash.Push(signal.Frame{
		Values: map[string]float32{"speed": 80, "accel_g": 2.5},
		At:     t0,
	})
	intentFallback, err := classifier.Classify(ctx, wCrash)
	if err != nil {
		t.Fatalf("classify fallback error: %v", err)
	}
	if intentFallback == nil || intentFallback.ClipName != "heavy_crash" {
		t.Fatalf("expected fallback RuleClassifier to identify heavy_crash, got %+v", intentFallback)
	}
}

func TestResolveLLMEndpoint(t *testing.T) {
	// 1. Explicit endpoint with path
	if got := ResolveLLMEndpoint("http://127.0.0.1:9995/completion"); got != "http://127.0.0.1:9995/completion" {
		t.Errorf("expected explicit URL, got %s", got)
	}

	// 2. Bare host:port normalization
	if got := ResolveLLMEndpoint("127.0.0.1:9995"); got != "http://127.0.0.1:9995/completion" {
		t.Errorf("expected normalized completion URL, got %s", got)
	}

	// 3. Environment variable override
	t.Setenv("DS5_LLM_ENDPOINT", "http://10.0.0.5:11434/api/generate")
	if got := ResolveLLMEndpoint(""); got != "http://10.0.0.5:11434/api/generate" {
		t.Errorf("expected env URL, got %s", got)
	}
}

func TestArbitrator_RollbackAndFinished(t *testing.T) {
	arb := NewArbitrator(1 * time.Second)
	t0 := time.Now()
	intent := &RadioIntent{
		ClipName: "crash",
		Priority: 100,
		Cooldown: 10 * time.Second,
	}

	// 1. Mark playing for 3s
	arb.MarkPlaying(intent, t0, 3*time.Second)

	// Cooldown and busy check
	if arb.ShouldPlay(intent, t0.Add(500*time.Millisecond)) {
		t.Fatal("expected intent to be rejected while busy")
	}

	// 2. Test MarkFinished resets busyUntil early
	arb.MarkFinished(intent, t0.Add(500*time.Millisecond))
	arb.mu.Lock()
	if !arb.busyUntil.Equal(t0.Add(500*time.Millisecond)) || arb.activePri != 0 {
		t.Fatalf("expected busyUntil to be reset to t0+500ms and activePri 0, got %v, %d", arb.busyUntil, arb.activePri)
	}
	arb.mu.Unlock()

	// 3. Test Rollback clears cooldown and busy completely
	arb.TryPlay(intent, t0.Add(10*time.Second), 3*time.Second)
	arb.Rollback(intent)

	arb.mu.Lock()
	if _, exists := arb.cooldowns[intent.ClipName]; exists {
		t.Fatal("expected cooldown to be removed on Rollback")
	}
	if !arb.busyUntil.IsZero() || arb.activePri != 0 || !arb.lastAny.IsZero() {
		t.Fatalf("expected busyUntil/activePri/lastAny to be zeroed on Rollback, got %v, %d, %v",
			arb.busyUntil, arb.activePri, arb.lastAny)
	}
	arb.mu.Unlock()
}

func TestArbitrator_AtomicTryPlay(t *testing.T) {
	arb := NewArbitrator(1 * time.Second)
	t0 := time.Now()
	intent := &RadioIntent{
		ClipName: "pace",
		Priority: 50,
		Cooldown: 5 * time.Second,
	}

	const nGoroutines = 50
	var successCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(nGoroutines)

	for i := 0; i < nGoroutines; i++ {
		go func() {
			defer wg.Done()
			if arb.TryPlay(intent, t0, 2*time.Second) {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 goroutine to win atomic TryPlay, got %d", successCount.Load())
	}
}

func TestPlayer_MissingClipFallback(t *testing.T) {
	tmpDir := t.TempDir()
	p, err := NewPlayer(Config{
		ClipsDir: tmpDir,
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}

	fbPath := p.resolveFallbackBeep()
	if _, err := os.Stat(fbPath); err != nil {
		t.Fatalf("fallback beep file missing: %v", err)
	}

	f, err := os.Open(fbPath)
	if err != nil {
		t.Fatalf("open fallback: %v", err)
	}
	defer f.Close()

	samples, rate, err := ReadWAV(f)
	if err != nil {
		t.Fatalf("ReadWAV on fallback: %v", err)
	}
	if len(samples) == 0 || rate != 48000 {
		t.Fatalf("invalid samples or rate: len=%d, rate=%d", len(samples), rate)
	}
}

type slowMockEngine struct {
	delay time.Duration
}

func (s *slowMockEngine) Generate(ctx context.Context, prompt string) (string, error) {
	select {
	case <-time.After(s.delay):
		return `{"clip": "test_clip", "priority": 50}`, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *slowMockEngine) Close() error { return nil }

func TestMonitor_AsyncWorker(t *testing.T) {
	p := &Player{logger: log.New(io.Discard, "", 0)}
	m := NewMonitor(p, log.New(io.Discard, "", 0))
	defer m.Close()

	slowEng := &slowMockEngine{delay: 50 * time.Millisecond}
	classifier := NewLLMClassifier(slowEng, PersonaF1Engineer, log.New(io.Discard, "", 0))
	defer classifier.Close()
	m.SetClassifier(classifier)

	ctx := context.Background()
	start := time.Now()

	for i := 0; i < 100; i++ {
		m.handleFrame(ctx, signal.Frame{
			Values: map[string]float32{"speed": float32(50 + i)},
			At:     start.Add(time.Duration(i) * 8 * time.Millisecond),
		})
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("handleFrame took too long (%v), async inference worker failed to decouple hot path", elapsed)
	}
}

func TestAcousticPipeline_WAVProcess(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src.wav")
	dstPath := filepath.Join(tmpDir, "dst.wav")

	sampleRate := 48000
	samples := make([]float32, 480)
	for i := range samples {
		samples[i] = float32(0.5 * math.Sin(2.0*math.Pi*440.0*float64(i)/float64(sampleRate)))
	}

	f, err := os.Create(srcPath)
	if err != nil {
		t.Fatalf("create src: %v", err)
	}
	if err := WriteWAV(f, samples, sampleRate); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}
	_ = f.Close()

	if err := ProcessWAV(srcPath, dstPath, StyleRadio); err != nil {
		t.Fatalf("ProcessWAV failed: %v", err)
	}

	dstF, err := os.Open(dstPath)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dstF.Close()

	outSamples, outRate, err := ReadWAV(dstF)
	if err != nil {
		t.Fatalf("ReadWAV on dst: %v", err)
	}
	if len(outSamples) != len(samples) || outRate != sampleRate {
		t.Fatalf("unexpected output: len=%d (want %d), rate=%d (want %d)",
			len(outSamples), len(samples), outRate, sampleRate)
	}
}
