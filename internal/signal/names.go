// Package signal defines the game-agnostic signal vocabulary and a time-aware
// signal store with per-signal shaping (deadzone, gamma, slew limit).
package signal

// Standard signal names. Every profile expression may reference any of these;
// a source only has to fill in the ones it can actually provide. Missing
// signals stay at 0 and report has() == 0, so a rule can decide to skip
// instead of silently driving the trigger to zero.
var Standard = []string{
	// Vehicle telemetry (racing games, sims, ...)
	"throttle", "brake", "clutch", "steer", "speed", "rpm", "gear",
	"slip", "lockup", "abs", "tc", "rumble", "accel_g", "lat_g", "surface",

	// Controller state (input-driven profiles, works in every game)
	"l2", "r2", "l2_full", "r2_full",
	"l1", "r1", "cross", "circle", "square", "triangle",
	"up", "down", "left", "right",
	"lx", "ly", "rx", "ry", "stick_l", "stick_r",
	"ps", "touchpad", "mute",

	// Generic escape hatches for profile-specific sources
	"custom0", "custom1", "custom2", "custom3",
}

var standard = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Standard))
	for _, n := range Standard {
		m[n] = struct{}{}
	}
	return m
}()

// IsStandard reports whether name belongs to the built-in vocabulary.
func IsStandard(name string) bool {
	_, ok := standard[name]
	return ok
}
