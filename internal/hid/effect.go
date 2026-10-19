// Package hid builds DualSense output reports and adaptive-trigger effect
// payloads. It knows nothing about games: callers hand it a Frame and get
// bytes for /dev/hidrawN (Linux) or a HID handle (Windows).
package hid

import "math"

// Mode is a DualSense adaptive-trigger effect mode (the first byte of the
// 11-byte per-trigger block).
//
// Lower modes (0x00-0x05) are the legacy "position based" family; 0x21-0x26
// are the richer "force feedback" family. The exact meaning of each parameter
// follows the Linux kernel driver (drivers/hid/hid-playstation.c) and the
// community documentation (flok/pydualsense). Values that feel wrong on real
// hardware are almost always a parameter-ordering issue, not a mode issue --
// use `ds5ctl bench` to sweep a mode interactively.
type Mode uint8

const (
	ModeOff           Mode = 0x00 // effect disabled
	ModeRigid         Mode = 0x01 // continuous resistance from a start position
	ModeSection       Mode = 0x02 // resistance only inside [start, end]
	ModeFeedback      Mode = 0x21 // resistance that follows trigger travel
	ModeWeapon        Mode = 0x22 // two-stage "gun trigger" break
	ModeVibration     Mode = 0x23 // vibration inside a travel window
	ModeSlopeFeedback Mode = 0x24 // resistance ramp between two positions
	ModeSlopeWeapon   Mode = 0x25 // two-stage break with amplitude ramp
	ModeMachine       Mode = 0x26 // periodic "machine gun" pulses
)

var modesByName = map[string]Mode{
	"off":            ModeOff,
	"rigid":          ModeRigid,
	"section":        ModeSection,
	"pulse":          ModeSection,
	"feedback":       ModeFeedback,
	"weapon":         ModeWeapon,
	"vibration":      ModeVibration,
	"slope_feedback": ModeSlopeFeedback,
	"slope":          ModeSlopeFeedback,
	"slope_weapon":   ModeSlopeWeapon,
	"machine":        ModeMachine,
}

// ParseMode resolves a profile-level effect name.
func ParseMode(name string) (Mode, bool) {
	m, ok := modesByName[name]
	return m, ok
}

func (m Mode) String() string {
	for n, v := range modesByName {
		if v == m && n != "pulse" && n != "slope" {
			return n
		}
	}
	return "unknown"
}

// paramNames maps a human-friendly parameter name to its index inside the
// 11-byte trigger block (index 0 is the mode byte, so payload params start at
// index 1, i.e. P[0]).
//
// Unknown names are rejected at profile load time; raw "p0".."p9" always work.
var paramNames = map[Mode]map[string]int{
	ModeRigid:         {"start": 0, "force": 1},
	ModeSection:       {"start": 0, "end": 1},
	ModeFeedback:      {"start": 0, "strength": 1, "force": 2},
	ModeWeapon:        {"start": 0, "amp_a": 1, "amp_b": 2, "freq": 3},
	ModeVibration:     {"start": 0, "amplitude": 1, "freq": 2},
	ModeSlopeFeedback: {"start": 0, "end": 1, "start_force": 2, "end_force": 3},
	ModeSlopeWeapon:   {"start": 0, "end": 1, "amp_a": 2, "amp_b": 3, "freq": 4},
	ModeMachine:       {"start": 0, "end": 1, "freq": 2, "period": 3},
}

// ParamIndex resolves a parameter name for a mode. It accepts "p0".."p9" for
// every mode so unfamiliar effects can still be driven from a profile.
func ParamIndex(m Mode, name string) (int, bool) {
	if len(name) == 2 && name[0] == 'p' && name[1] >= '0' && name[1] <= '9' {
		return int(name[1] - '0'), true
	}
	mn, ok := paramNames[m]
	if !ok {
		return 0, false
	}
	i, ok := mn[name]
	return i, ok
}

// ParamNames lists the semantic parameter names accepted by a mode.
func ParamNames(m Mode) []string {
	mn := paramNames[m]
	out := make([]string, 0, len(mn))
	for n := range mn {
		out = append(out, n)
	}
	return out
}

// forceParams lists the parameter indices that encode "how hard the trigger
// pushes back". They are scaled when an effect decays, so a release fades
// instead of snapping the trigger open.
var forceParams = map[Mode][]int{
	ModeRigid:         {1},
	ModeFeedback:      {2},
	ModeWeapon:        {1, 2},
	ModeVibration:     {1},
	ModeSlopeFeedback: {2, 3},
	ModeSlopeWeapon:   {2, 3},
	ModeMachine:       {1},
	ModeSection:       {},
	ModeOff:           {},
}

// Trigger is one adaptive-trigger effect: a mode plus up to 10 parameters.
type Trigger struct {
	Mode Mode
	P    [10]byte
}

// Off reports whether the trigger is released.
func (t Trigger) IsOff() bool { return t.Mode == ModeOff }

// Set writes parameter i, clamping to 0..255.
func (t *Trigger) Set(i int, v float64) {
	if i < 0 || i >= len(t.P) {
		return
	}
	t.P[i] = byte(math.Round(clamp(v, 0, 255)))
}

// Scaled returns a copy with every force parameter multiplied by f (0..1).
// Used for fade-out; mode and geometry parameters are preserved.
func (t Trigger) Scaled(f float64) Trigger {
	if f >= 1 {
		return t
	}
	if f <= 0 {
		return Trigger{}
	}
	out := t
	for _, i := range forceParams[t.Mode] {
		out.P[i] = byte(math.Round(float64(out.P[i]) * f))
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
