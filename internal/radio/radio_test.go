package radio

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
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
