// Package signal defines the game-agnostic signal vocabulary and a time-aware
// signal store with per-signal shaping (deadzone, gamma, slew limit).
package signal

// Standard signal names. Every profile expression may reference any of these;
// a source only has to fill in the ones it can actually provide. Missing
// signals stay at 0 and report has() == 0, so a rule can decide to skip
// instead of silently driving the trigger to zero.
//
// APPEND ONLY: the bridge wire format sends ids (indices into this slice), so
// inserting a name in the middle silently breaks compatibility with an already
// deployed ds5bridge.exe. New names go at the very end of the slice.
var Standard = []string{
	// Vehicle telemetry (racing games, sims, ...)
	"throttle", "brake", "clutch", "steer", "speed", "rpm", "gear",
	"slip", "lockup", "abs", "tc", "rumble", "accel_g", "lat_g", "surface",
	"slip_fl", "slip_fr", "slip_rl", "slip_rr",

	// Controller state (input-driven profiles, works in every game)
	"l2", "r2", "l2_full", "r2_full",
	"l1", "r1", "cross", "circle", "square", "triangle",
	"up", "down", "left", "right",
	"lx", "ly", "rx", "ry", "stick_l", "stick_r",
	"ps", "touchpad", "mute",

	// Generic escape hatches for profile-specific sources
	"custom0", "custom1", "custom2", "custom3",

	// Controller sensors (appended, see the note above)
	"gyro_x", "gyro_y", "gyro_z", // full-scale normalised -1..1
	"accel_x", "accel_y", "accel_z",
	"touch_x", "touch_y", "touch_active", // touchpad contact, 0..1
	"battery", "charging", // battery 0..1 (10% steps)

	// Audio envelope (source/audio): 0..1, already smoothed and normalised.
	"bass", "mid", "loudness",

	// Force feedback captured from a virtual pad (source/ffb): 0..1 per motor.
	"ffb_left", "ffb_right",

	// Transient detector (source/audio): 0..1, spikes on onsets (gunshots,
	// impacts) regardless of how loud the background bed is.
	"onset",

	// Audio AI event detector signals (ambient surface & sound events)
	"surface_water", "surface_metal", "surface_wood", "surface_gravel",
	"audio_gunshot", "audio_explosion", "audio_impact",

	// Radio / pit strategy signals
	"box", "pit",

	// Cross-genre companion signals (Roguelike, Souls-like, Tactical FPS)
	"hp", "stamina", "mana", "hazard",
}

var standardIDs = func() map[string]uint8 {
	m := make(map[string]uint8, len(Standard))
	for i, n := range Standard {
		m[n] = uint8(i)
	}
	return m
}()

// IsStandard reports whether name belongs to the built-in vocabulary.
func IsStandard(name string) bool {
	_, ok := standardIDs[name]
	return ok
}

// ID returns the wire id of a signal name. The bridge sends ids instead of
// strings, so both sides must be built from the same vocabulary.
func ID(name string) (uint8, bool) {
	id, ok := standardIDs[name]
	return id, ok
}

// Name returns the signal name for a wire id, or "" if unknown.
func Name(id uint8) string {
	if int(id) < len(Standard) {
		return Standard[id]
	}
	return ""
}
