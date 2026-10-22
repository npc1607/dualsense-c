package radio

import (
	"sync"
	"time"
)

// Arbitrator manages radio speech discipline, per-intent cooldown timers,
// global spacing intervals, and priority-based preemption.
type Arbitrator struct {
	mu         sync.Mutex
	cooldowns  map[string]time.Time
	lastAny    time.Time
	minSpacing time.Duration
	activePri  int
	activeClip string
	busyUntil  time.Time
}

// NewArbitrator creates a new radio discipline arbitrator with the given minimum speech spacing.
func NewArbitrator(minSpacing time.Duration) *Arbitrator {
	return &Arbitrator{
		cooldowns:  make(map[string]time.Time),
		minSpacing: minSpacing,
	}
}

// TryPlay atomically evaluates whether an intent is allowed to play at the given time,
// and if permitted, claims the channel, starts cooldowns, and updates arbitrator state.
func (a *Arbitrator) TryPlay(intent *RadioIntent, now time.Time, estDuration time.Duration) bool {
	if intent == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	// 1. Cooldown check: per-intent cooldown
	if lastFired, ok := a.cooldowns[intent.ClipName]; ok {
		if now.Sub(lastFired) < intent.Cooldown {
			return false
		}
	}

	// 2. Active busy check: can only play if idle or preemption threshold reached
	isPreemption := false
	if now.Before(a.busyUntil) {
		if intent.Priority >= a.activePri+30 {
			isPreemption = true
		} else {
			return false
		}
	}

	// 3. Spacing check: quiet gap after previous transmission started
	if !isPreemption && !a.lastAny.IsZero() && now.Sub(a.lastAny) < a.minSpacing {
		return false
	}

	a.cooldowns[intent.ClipName] = now
	a.lastAny = now
	a.activePri = intent.Priority
	a.activeClip = intent.ClipName
	a.busyUntil = now.Add(estDuration)
	return true
}

// ShouldPlay checks whether an intent is permitted to be broadcast at the given time.
func (a *Arbitrator) ShouldPlay(intent *RadioIntent, now time.Time) bool {
	if intent == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	// 1. Cooldown check: per-intent cooldown
	if lastFired, ok := a.cooldowns[intent.ClipName]; ok {
		if now.Sub(lastFired) < intent.Cooldown {
			return false
		}
	}

	// 2. Active busy check: can only play if idle or preemption threshold reached
	isPreemption := false
	if now.Before(a.busyUntil) {
		if intent.Priority >= a.activePri+30 {
			isPreemption = true
		} else {
			return false
		}
	}

	// 3. Spacing check: quiet gap after previous transmission started
	if !isPreemption && !a.lastAny.IsZero() && now.Sub(a.lastAny) < a.minSpacing {
		return false
	}

	return true
}

// MarkPlaying registers that an intent has started playing, updating busy state and cooldown timers.
func (a *Arbitrator) MarkPlaying(intent *RadioIntent, now time.Time, estDuration time.Duration) {
	if intent == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cooldowns[intent.ClipName] = now
	a.lastAny = now
	a.activePri = intent.Priority
	a.activeClip = intent.ClipName
	a.busyUntil = now.Add(estDuration)
}

// MarkFinished registers that an intent has finished playing, resetting busyUntil and activePri early.
func (a *Arbitrator) MarkFinished(intent *RadioIntent, now time.Time) {
	if intent == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.activeClip == intent.ClipName {
		a.busyUntil = now
		a.activePri = 0
		a.activeClip = ""
	}
}

// Rollback clears the cooldown and channel locks for an intent if its playback failed or was rejected.
func (a *Arbitrator) Rollback(intent *RadioIntent) {
	if intent == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.cooldowns, intent.ClipName)
	if a.activeClip == intent.ClipName {
		a.busyUntil = time.Time{}
		a.activePri = 0
		a.activeClip = ""
		a.lastAny = time.Time{}
	}
}

// Reset clears all cooldown and busy states.
func (a *Arbitrator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cooldowns = make(map[string]time.Time)
	a.lastAny = time.Time{}
	a.activePri = 0
	a.activeClip = ""
	a.busyUntil = time.Time{}
}
