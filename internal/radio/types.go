package radio

import (
	"context"
	"time"
)

// IntentCategory classifies the nature of a radio message.
type IntentCategory string

const (
	CategoryWarning    IntentCategory = "warning"
	CategoryMechanical IntentCategory = "mechanical"
	CategoryStrategy   IntentCategory = "strategy"
	CategoryPace       IntentCategory = "pace"
)

// RadioIntent defines the structured decision output from an inference engine.
type RadioIntent struct {
	ID       string
	Category IntentCategory
	Priority int           // Preemption priority: Crash(100) > Mech(70) > Warning(50) > Pace(30)
	ClipName string        // Target audio asset filename without extension
	Text     string        // Natural language voice transcript (for TTS / logging)
	Cooldown time.Duration // Cooldown interval for this intent
}

// Classifier is the interface for telemetry-driven radio inference engines.
type Classifier interface {
	Classify(ctx context.Context, w *TelemetryWindow) (*RadioIntent, error)
	Close() error
}
