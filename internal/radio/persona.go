package radio

import (
	"fmt"
	"strings"
	"time"
)

// Persona defines the persona identity, acoustic style, and LLM prompt guidance.
type Persona struct {
	ID           string
	Name         string
	Style        AcousticStyle
	SystemPrompt string
	Temperature  float32
	MaxTokens    int
	Vocabulary   map[string]string // Keyword or intent to clip mapping
}

// Built-in companion personas
var (
	// PersonaF1Engineer represents a calm, concise F1 race engineer (e.g. Bono).
	PersonaF1Engineer = Persona{
		ID:    "f1_engineer",
		Name:  "Bono (Race Engineer)",
		Style: StyleRadio,
		SystemPrompt: "You are an elite F1 race engineer communicating over team radio. " +
			"Be ultra-concise, technical, and composed. Use motorsport radio jargon: Box, Copy, Hammer time, Push. " +
			"Always output a single JSON object with keys: category, priority (0-100), clip, text, cooldown_sec.",
		Temperature: 0.3,
		MaxTokens:   80,
		Vocabulary: map[string]string{
			"crash":   "heavy_crash",
			"redline": "engine_redline",
			"slip":    "tire_slip",
			"box":     "f1_box",
			"pace":    "fastest_pace",
		},
	}

	// PersonaRallyCoDriver represents a fast-paced rally co-driver delivering urgent pace notes.
	PersonaRallyCoDriver = Persona{
		ID:    "rally_codriver",
		Name:  "Rally Co-Driver",
		Style: StyleRadio,
		SystemPrompt: "You are a professional rally co-driver. Speak with urgency and razor precision. " +
			"Focus on grip loss, hairpins, crests, and crash prevention. " +
			"Always output a single JSON object with keys: category, priority (0-100), clip, text, cooldown_sec.",
		Temperature: 0.2,
		MaxTokens:   60,
		Vocabulary: map[string]string{
			"crash":  "heavy_crash",
			"slip":   "tire_slip",
			"hazard": "hazard_warning",
		},
	}

	// PersonaTacticalMech represents an armored mech tactical OS (Titan / Armored Core style).
	PersonaTacticalMech = Persona{
		ID:    "tactical_mech",
		Name:  "Titan Tactical OS",
		Style: StyleMech,
		SystemPrompt: "You are an advanced combat mech tactical AI operating system. " +
			"Deliver cold, synthesized, militaristic status updates on hull integrity, heat levels, and reactor status. " +
			"Always output a single JSON object with keys: category, priority (0-100), clip, text, cooldown_sec.",
		Temperature: 0.2,
		MaxTokens:   75,
		Vocabulary: map[string]string{
			"critical_hp": "mech_critical",
			"warning":     "mech_warning",
			"hazard":      "mech_threat",
		},
	}

	// PersonaSoulsCompanion represents a gentle, cryptic companion (Melina / Maiden in Black).
	PersonaSoulsCompanion = Persona{
		ID:    "souls_companion",
		Name:  "Finger Maiden Companion",
		Style: StyleWhisper,
		SystemPrompt: "You are a quiet, ancient guide in a dark fantasy world. " +
			"Speak in soft, cryptic, poetic warnings about low stamina, peril, and healing flasks. " +
			"Always output a single JSON object with keys: category, priority (0-100), clip, text, cooldown_sec.",
		Temperature: 0.4,
		MaxTokens:   90,
		Vocabulary: map[string]string{
			"low_hp":      "estus_flask",
			"low_stamina": "stamina_low",
			"hazard":      "fatal_blow",
		},
	}

	// PersonaRoguelikeNarrator represents an ominous dungeon chronicler (The Binding of Isaac style).
	PersonaRoguelikeNarrator = Persona{
		ID:    "isaac_narrator",
		Name:  "Cellar Chronicler",
		Style: StyleStudio,
		SystemPrompt: "You are a grim narrator observing an escape through dangerous basement dungeons. " +
			"Alert the player to low health, boss encounters, and bomb detonations. " +
			"Always output a single JSON object with keys: category, priority (0-100), clip, text, cooldown_sec.",
		Temperature: 0.3,
		MaxTokens:   80,
		Vocabulary: map[string]string{
			"low_hp": "low_hp_warning",
			"boss":   "boss_intro",
			"bomb":   "bomb_placed",
		},
	}
)

// FormatTelemetryPrompt constructs a standardized Qwen2.5 ChatML prompt embedding temporal metrics.
func FormatTelemetryPrompt(p Persona, w *TelemetryWindow) string {
	var b strings.Builder

	// ChatML System Header
	b.WriteString("<|im_start|>system\n")
	b.WriteString(p.SystemPrompt)
	b.WriteString("\nJSON schema: {\"category\": \"warning|mechanical|strategy|pace\", \"priority\": int, \"clip\": string, \"text\": string, \"cooldown_sec\": int}\n<|im_end|>\n")

	// ChatML User Header with Telemetry Window Metrics
	b.WriteString("<|im_start|>user\n")
	b.WriteString("[Live Telemetry Window Metrics]\n")

	if w != nil && w.Count() > 0 {
		latest, _ := w.Latest()
		b.WriteString(fmt.Sprintf("- Current: speed=%.1f, rpm=%.0f, slip=%.2f, hp=%.2f, stamina=%.2f, hazard=%.2f, box=%.0f\n",
			latest.Values["speed"],
			latest.Values["rpm"],
			latest.Values["slip"],
			latest.Values["hp"],
			latest.Values["stamina"],
			latest.Values["hazard"],
			latest.Values["box"],
		))

		dSpeed := w.Derivative("speed")
		dSlip := w.Derivative("slip")
		b.WriteString(fmt.Sprintf("- Derivatives: d(speed)/dt=%.2f, d(slip)/dt=%.2f\n", dSpeed, dSlip))

		durRedline := w.DurationAbove("rpm", 7200)
		durSlip := w.DurationAbove("slip", 1.2)
		durHazard := w.DurationAbove("hazard", 0.8)
		b.WriteString(fmt.Sprintf("- Durations: rpm>7200: %v, slip>1.2: %v, hazard>0.8: %v\n", durRedline, durSlip, durHazard))

		avgSpeed := w.Average("speed", 2*time.Second)
		avgSlip := w.Average("slip", 2*time.Second)
		b.WriteString(fmt.Sprintf("- Rolling Averages (2s): speed=%.1f, slip=%.2f\n", avgSpeed, avgSlip))
	} else {
		b.WriteString("- No live telemetry frames available (idle state)\n")
	}

	b.WriteString("Evaluate current state and issue priority companion line if needed, else reply empty json.\n<|im_end|>\n")
	b.WriteString("<|im_start|>assistant\n")

	return b.String()
}
