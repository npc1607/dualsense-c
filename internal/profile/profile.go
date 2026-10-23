// Package profile loads declarative per-game profiles: which source to read,
// how to shape signals, and which rules turn signals into trigger/rumble
// effects. Adapting a new game should be a YAML edit, not a rebuild.
package profile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration accepts "120ms", "1.5s" or a bare integer (milliseconds).
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag == "!!int" {
		ms, err := strconv.Atoi(node.Value)
		if err != nil {
			return err
		}
		*d = Duration(time.Duration(ms) * time.Millisecond)
		return nil
	}
	v, err := time.ParseDuration(strings.TrimSpace(node.Value))
	if err != nil {
		return fmt.Errorf("profile: bad duration %q: %w", node.Value, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// Defaults applied when a profile omits a field.
const (
	DefaultSignalTTL = 400 * time.Millisecond
	DefaultRelease   = 250 * time.Millisecond
	DefaultRateHz    = 125
)

// Profile is one game (or one game-agnostic behaviour) definition.
type Profile struct {
	Name        string     `yaml:"name"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Source      Source     `yaml:"source"`
	Shape       Shapers    `yaml:"shape"`
	Safety      Safety     `yaml:"safety"`
	Extra       []string   `yaml:"signals"` // extra signal names this profile may use
	Rules       []Rule     `yaml:"rules"`
	Haptics     *Haptics   `yaml:"haptics"` // optional voice-coil output
	Aim         *AimConfig `yaml:"aim"`
	AimAssist   *AimConfig `yaml:"aim_assist"` // Backward compatible alias for fps-aim.yaml
}

// AimConfig defines stick shaping, gyro aiming, and anti-recoil enhancements.
type AimConfig struct {
	Curve      AimCurveConfig      `yaml:"curve"`
	Filter     AimFilterConfig     `yaml:"filter"`
	Gyro       AimGyroConfig       `yaml:"gyro"`
	AntiRecoil AimAntiRecoilConfig `yaml:"anti_recoil"`
}

type AimCurveConfig struct {
	Type          string  `yaml:"type"`
	Exponent      float64 `yaml:"exponent"`
	MidPoint      float64 `yaml:"mid_point"`
	InnerDeadzone float64 `yaml:"inner_deadzone"`
	OuterBoost    float64 `yaml:"outer_boost"`
}

type AimFilterConfig struct {
	Type      string  `yaml:"type"`
	MinCutoff float64 `yaml:"min_cutoff"`
	Beta      float64 `yaml:"beta"`
	DCutoff   float64 `yaml:"d_cutoff"`
}

type AimGyroConfig struct {
	Enabled          bool    `yaml:"enabled"`
	Trigger          string  `yaml:"trigger"`
	TriggerThreshold float32 `yaml:"trigger_threshold"`
	NaturalAngleDeg  float64 `yaml:"natural_angle_deg"`
	DeadzoneDPS      float64 `yaml:"deadzone_dps"`
	SensYaw          float64 `yaml:"sens_yaw"`
	SensPitch        float64 `yaml:"sens_pitch"`
	InvertPitch      bool    `yaml:"invert_pitch"`
}

type AimAntiRecoilConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Trigger      string   `yaml:"trigger"`
	StartDelay   Duration `yaml:"start_delay"`
	BurstForce   float64  `yaml:"burst_force"`
	BurstDecay   float64  `yaml:"burst_decay"`
	SustainForce float64  `yaml:"sustain_force"`
	JitterNoise  float64  `yaml:"jitter_noise"`
}

// EffectiveAim returns the active aim configuration (Aim or AimAssist).
func (p *Profile) EffectiveAim() *AimConfig {
	if p.Aim != nil {
		return p.Aim
	}
	return p.AimAssist
}

// Haptics configures the optional voice-coil (audio) output.
//
// The HID rumble bytes can only express an amplitude plus one modulation
// frequency, which is why fine texture feels coarse on them. The controller is
// also a 4-channel USB sound card whose channels 3/4 drive the same coils, so
// streaming PCM there gives real waveforms. Rules write that output through the
// `haptic_left` / `haptic_right` targets.
type Haptics struct {
	// Enabled defaults to true when the block is present.
	Enabled *bool `yaml:"enabled"`
	// Target is a PipeWire sink name, or "auto" to detect the DualSense.
	Target string `yaml:"target"`
	// CarrierHz is the fundamental of the synthesised waveform (default 55).
	CarrierHz float64 `yaml:"carrier_hz"`
	// Gain scales the 0..255 rule output into PCM amplitude (default 0.55).
	Gain float64 `yaml:"gain"`
	// Command overrides the playback command template.
	Command string `yaml:"command"`
	// Mode is "synth" (fixed carrier) or "passthrough" (filter the game audio
	// and send it to the coils — real HD texture).
	Mode string `yaml:"mode"`
	// Source is the monitor to capture in passthrough mode ("auto" = default).
	Source string `yaml:"source"`
	// LowHz/HighHz bound the band sent to the coils (default 25–180).
	LowHz  float64 `yaml:"low_hz"`
	HighHz float64 `yaml:"high_hz"`
}

// IsEnabled reports whether haptic output should be opened.
func (h *Haptics) IsEnabled() bool {
	return h != nil && (h.Enabled == nil || *h.Enabled)
}

// Shapers maps a signal name to its shaping parameters.
type Shapers map[string]Shape

// Shape mirrors signal.Shaper in YAML form (kept separate so the profile
// package stays independent of the runtime types).
type Shape struct {
	Deadzone float64 `yaml:"deadzone"`
	Gamma    float64 `yaml:"gamma"`
	SlewUp   float64 `yaml:"slew_up"`
	SlewDown float64 `yaml:"slew_down"`
}

// Source selects the adapter that feeds signals into the rules.
type Source struct {
	Type   string         `yaml:"type"`
	RateHz int            `yaml:"rate_hz"`
	Params map[string]any `yaml:",inline"`
}

// Safety controls how the controller behaves when data stops arriving.
type Safety struct {
	// SignalTTL is how long a signal stays usable after its source stops
	// updating; older values are treated as absent.
	SignalTTL Duration `yaml:"signal_ttl"`
	// Release is how long an active effect fades out once no rule drives it.
	// A non-zero value prevents the trigger from snapping open when a game
	// closes or a source stalls.
	Release Duration `yaml:"release"`
	// StatusLightbar paints the lightbar green (live) / yellow (idle) when no
	// rule targets the lightbar. Enabled by default.
	StatusLightbar *bool `yaml:"status_lightbar"`
}

// Rule is one declarative mapping from signals to an effect.
//
//	target: l2 | r2 | rumble_left | rumble_right | haptic_left | haptic_right |
//	        lightbar | player_leds | mute_led
//	effect: off | rigid | section | feedback | weapon | vibration |
//	        slope_feedback | slope_weapon | machine      (triggers only)
//	when:   optional expression, the rule only applies while it is true
//	params: effect parameters, each value an expression
//	value:  numeric expression (rumble / player LEDs)
//	hold:   latch the rule for this long after `when` stops being true
type Rule struct {
	Name     string            `yaml:"name"`
	When     string            `yaml:"when"`
	Target   string            `yaml:"target"`
	Effect   string            `yaml:"effect"`
	Params   map[string]string `yaml:"params"`
	Value    string            `yaml:"value"`
	Priority int               `yaml:"priority"`
	Hold     Duration          `yaml:"hold"`
	Modulate *Modulate         `yaml:"modulate"`
	Clip     string            `yaml:"clip"`     // Audio asset name without extension
	Cooldown Duration          `yaml:"cooldown"` // Minimum quiet time between firings
	Text     string            `yaml:"text"`     // Natural language transcript or hint
}

// Modulate applies low-frequency amplitude modulation (a rumble buzz).
type Modulate struct {
	Type  string  `yaml:"type"` // sine | square
	Hz    float64 `yaml:"hz"`
	Depth float64 `yaml:"depth"`
}

// TTL returns the effective signal TTL.
func (p *Profile) TTL() time.Duration {
	if p.Safety.SignalTTL > 0 {
		return p.Safety.SignalTTL.Duration()
	}
	return DefaultSignalTTL
}

// ReleaseTime returns the effective fade-out duration.
func (p *Profile) ReleaseTime() time.Duration {
	if p.Safety.Release > 0 {
		return p.Safety.Release.Duration()
	}
	return DefaultRelease
}

// StatusLight reports whether the daemon should paint a status lightbar.
func (p *Profile) StatusLight() bool {
	if p.Safety.StatusLightbar == nil {
		return true
	}
	return *p.Safety.StatusLightbar
}

// RateHz returns the source sampling rate.
func (p *Profile) RateHz() int {
	if p.Source.RateHz > 0 {
		return p.Source.RateHz
	}
	return DefaultRateHz
}

// Signals returns every signal name the profile may reference: the standard
// vocabulary plus the profile's extras.
func (p *Profile) Signals() []string {
	return append([]string{}, p.Extra...)
}

// HasTarget reports whether any rule drives the given target. The daemon uses
// it to decide whether it may paint a status lightbar.
func (p *Profile) HasTarget(t string) bool {
	for _, r := range p.Rules {
		if r.Target == t {
			return true
		}
	}
	return false
}

// Validate performs structural checks that do not need the expression engine.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile: name is required")
	}
	if p.Source.Type == "" {
		return fmt.Errorf("profile %s: source.type is required", p.Name)
	}

	// Non-empty rule names must be unique: StableRuleID is derived from the
	// name, so duplicates would share one telemetry id and one reload timer
	// slot. Anonymous rules (Name == "") stay allowed and keep their rules[i]
	// label.
	seen := make(map[string]int, len(p.Rules))
	for i, r := range p.Rules {
		if r.Name == "" {
			continue
		}
		if first, ok := seen[r.Name]; ok {
			return fmt.Errorf(
				"profile %s: duplicate rule name %q (rules[%d] and rules[%d])",
				p.Name, r.Name, first, i)
		}
		seen[r.Name] = i
	}

	for i, r := range p.Rules {
		label := r.Name
		if label == "" {
			label = fmt.Sprintf("rules[%d]", i)
		}
		if r.Target == "" {
			return fmt.Errorf("profile %s: %s: target is required", p.Name, label)
		}
		switch r.Target {
		case "l2", "r2":
			if r.Effect == "" {
				return fmt.Errorf("profile %s: %s: effect is required for target %s", p.Name, label, r.Target)
			}
		case "rumble_left", "rumble_right", "haptic_left", "haptic_right",
			"player_leds", "mute_led":
			if r.Value == "" {
				return fmt.Errorf("profile %s: %s: value is required for target %s", p.Name, label, r.Target)
			}
		case "lightbar":
			for _, k := range []string{"r", "g", "b"} {
				if _, ok := r.Params[k]; !ok {
					return fmt.Errorf("profile %s: %s: lightbar needs params.r/g/b", p.Name, label)
				}
			}
		case "speak", "radio":
			if r.Clip == "" && r.Effect == "" && r.Value == "" {
				return fmt.Errorf("profile %s: %s: target %s requires clip, effect, or value", p.Name, label, r.Target)
			}
		default:
			return fmt.Errorf("profile %s: %s: unknown target %q", p.Name, label, r.Target)
		}
		if r.Effect != "" && r.Target != "speak" && r.Target != "radio" {
			switch r.Effect {
			case "off", "rigid", "section", "pulse", "feedback", "weapon",
				"vibration", "slope_feedback", "slope", "slope_weapon", "machine":
			default:
				return fmt.Errorf("profile %s: %s: unknown effect %q", p.Name, label, r.Effect)
			}
		}
		if r.Modulate != nil {
			switch r.Modulate.Type {
			case "", "sine", "square":
			default:
				return fmt.Errorf("profile %s: %s: unknown modulate type %q", p.Name, label, r.Modulate.Type)
			}
			if r.Modulate.Hz <= 0 {
				return fmt.Errorf("profile %s: %s: modulate.hz must be > 0", p.Name, label)
			}
		}
	}
	if aim := p.EffectiveAim(); aim != nil {
		if aim.Curve.Type != "" {
			switch aim.Curve.Type {
			case "linear", "power", "sigmoid", "dual_zone":
			default:
				return fmt.Errorf("profile %s: unknown aim curve type %q", p.Name, aim.Curve.Type)
			}
		}
		if aim.Filter.Type != "" {
			switch aim.Filter.Type {
			case "none", "ema", "one_euro":
			default:
				return fmt.Errorf("profile %s: unknown aim filter type %q", p.Name, aim.Filter.Type)
			}
		}
		if aim.Gyro.Enabled && aim.Gyro.Trigger != "" {
			switch aim.Gyro.Trigger {
			case "l2_hold", "touch", "always":
			default:
				return fmt.Errorf("profile %s: unknown aim gyro trigger %q", p.Name, aim.Gyro.Trigger)
			}
		}
		if aim.AntiRecoil.Enabled {
			if aim.AntiRecoil.BurstDecay < 0 {
				return fmt.Errorf("profile %s: burst_decay must be >= 0", p.Name)
			}
			if aim.AntiRecoil.BurstForce < 0 {
				return fmt.Errorf("profile %s: burst_force must be >= 0", p.Name)
			}
		}
	}
	return nil
}

// DecodeOptions controls profile YAML decoding. The zero value is the safe
// default: unknown keys are rejected.
//
// Lenient restores the legacy yaml.Unmarshal behaviour (unknown keys are
// silently dropped). It exists only as a migration escape hatch for
// `-strict-profile=false`, never as the default.
//
// Strictness covers top-level profile keys, rule keys and the nested structs
// reachable from them. It deliberately does not cover source parameters:
// Source.Params is `yaml:",inline"`, so an unknown source key is stored in the
// map and validated by the source adapter that understands it.
type DecodeOptions struct {
	Lenient bool
}

// firstDecodeOptions applies the strict default when the variadic option is
// omitted. Callers may pass at most one value; later values are ignored.
func firstDecodeOptions(opts []DecodeOptions) DecodeOptions {
	if len(opts) == 0 {
		return DecodeOptions{}
	}
	return opts[0]
}

// decodeProfile decodes one YAML document using opts.
func decodeProfile(raw []byte, what string, opts DecodeOptions) (*Profile, error) {
	var p Profile
	if opts.Lenient {
		if err := yaml.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("profile %s: %w", what, err)
		}
		return &p, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("profile %s: %w", what, err)
	}
	return &p, nil
}

// LoadFile reads and validates a profile from disk.
func LoadFile(path string, opts ...DecodeOptions) (*Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	p, err := decodeProfile(raw, path, firstDecodeOptions(opts))
	if err != nil {
		return nil, err
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Dir lists profiles in dir, sorted by name.
func Dir(dir string, opts ...DecodeOptions) ([]*Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p, err := LoadFile(filepath.Join(dir, e.Name()), opts...)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
