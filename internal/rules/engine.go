// Package rules compiles a profile's declarative rules into an expression
// engine and evaluates them once per output tick.
package rules

import (
	"cmp"
	"fmt"
	"log"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
)

type target string

const (
	RuleStateFalse    uint8 = 0
	RuleStateTrue     uint8 = 1
	RuleStateHold     uint8 = 2
	RuleStateStale    uint8 = 3
	RuleStateCooldown uint8 = 4
	RuleStateError    uint8 = 5
)

// RuleTelemetry exposes rule execution decisions and timing for UI inspection.
type RuleTelemetry struct {
	RuleID     uint16 `json:"rule_id"`
	Name       string `json:"name"`
	Target     string `json:"target"`
	Priority   int16  `json:"priority"`
	State      uint8  `json:"state"`
	LastFireMS uint32 `json:"last_fire_ms"`
	UntilMS    uint32 `json:"until_ms"`
}

const (
	targetL2          target = "l2"
	targetR2          target = "r2"
	targetRumbleLeft  target = "rumble_left"
	targetRumbleRight target = "rumble_right"
	targetHapticLeft  target = "haptic_left"
	targetHapticRight target = "haptic_right"
	targetLightbar    target = "lightbar"
	targetPlayerLEDs  target = "player_leds"
	targetMuteLED     target = "mute_led"
	targetSpeak       target = "speak"
	targetRadio       target = "radio"
)

// SpeechEvent represents a triggered companion speech line or radio broadcast.
type SpeechEvent struct {
	Name     string
	Target   string // "speak" | "radio"
	Clip     string
	Priority int
	Text     string
	At       time.Time
}

type rule struct {
	name      string
	target    target
	mode      hid.Mode
	priority  int
	when      *vm.Program
	value     *vm.Program
	params    map[int]*vm.Program
	rgb       [3]*vm.Program
	modulate  *profile.Modulate
	hold      time.Duration
	until     time.Time
	clip      string
	cooldown  time.Duration
	lastFired time.Time
	text      string
	wasActive bool
	lastState uint8
}

// Engine turns the current signal values into one controller frame.
type Engine struct {
	p     *profile.Profile
	set   *signal.Set
	log   *log.Logger
	rules []*rule
	names []string

	env   map[string]any
	prev  map[string]float64
	cur   map[string]float64
	fresh map[string]bool
	now   time.Time
	start time.Time
	vmr   vm.VM

	lastTrigger map[target]hid.Trigger
	lastRumble  map[target]uint8
	releaseAt   map[target]time.Time

	errCount map[string]int
	errLogAt time.Time

	speechEvents  []SpeechEvent
	speechMu      sync.Mutex
	speechHandler func(SpeechEvent)
	mu            sync.RWMutex
}

// New compiles every rule in the profile. Compilation is strict: unknown
// signal names, unknown functions and type errors are reported immediately, so
// a typo in a profile fails at startup instead of silently doing nothing.
func New(p *profile.Profile, set *signal.Set, logger *log.Logger) (*Engine, error) {
	if logger == nil {
		logger = log.Default()
	}
	e := &Engine{
		p:           p,
		set:         set,
		log:         logger,
		names:       append(append([]string{}, signal.Standard...), p.Extra...),
		prev:        map[string]float64{},
		cur:         map[string]float64{},
		fresh:       map[string]bool{},
		start:       time.Now(),
		lastTrigger: map[target]hid.Trigger{},
		lastRumble:  map[target]uint8{},
		releaseAt:   map[target]time.Time{},
		errCount:    map[string]int{},
	}
	e.env = make(map[string]any, len(e.names)+10)
	for _, n := range e.names {
		e.env[n] = float64(0)
	}
	e.installHelpers()

	for i, pr := range p.Rules {
		r, err := e.compileRule(pr, i)
		if err != nil {
			return nil, err
		}
		e.rules = append(e.rules, r)
	}
	slices.SortStableFunc(e.rules, func(a, b *rule) int { return cmp.Compare(a.priority, b.priority) })
	return e, nil
}

func (e *Engine) installHelpers() {
	// Predicates return bool so they compose with && / || in `when`.
	// For arithmetic use a ternary: `pressed("x") ? 1 : 0`.
	e.env["pressed"] = func(name string) bool {
		return e.prev[name] < 0.5 && e.cur[name] >= 0.5
	}
	e.env["released"] = func(name string) bool {
		return e.prev[name] >= 0.5 && e.cur[name] < 0.5
	}
	e.env["held"] = func(name string) bool {
		return e.cur[name] >= 0.5
	}
	// crossed(name, level) is the edge detector with an explicit threshold:
	// pressed() is fixed at 0.5, which is too high for quiet game audio.
	e.env["crossed"] = func(name string, level float64) bool {
		return e.prev[name] < level && e.cur[name] >= level
	}
	e.env["has"] = func(name string) bool {
		return e.fresh[name]
	}
	e.env["clamp"] = func(v, lo, hi float64) float64 {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	e.env["lerp"] = func(a, b, t float64) float64 { return a + (b-a)*clamp01(t) }
	e.env["smoothstep"] = func(edge0, edge1, x float64) float64 {
		if edge1 == edge0 {
			return 0
		}
		t := clamp01((x - edge0) / (edge1 - edge0))
		return t * t * (3 - 2*t)
	}
	e.env["sec"] = func() float64 { return e.now.Sub(e.start).Seconds() }
	// absf is absolute value. It cannot be called "abs": that name is already
	// taken by the ABS-intervention signal, and signal names win because they
	// are bound into the same environment.
	e.env["absf"] = math.Abs
	// The README advertises these as builtins, but expr-lang only ships
	// min/max/round; the rest have to be bound here.
	e.env["sqrt"] = func(v float64) float64 {
		if v < 0 {
			return 0
		}
		return math.Sqrt(v)
	}
	e.env["pow"] = math.Pow
	e.env["ceil"] = math.Ceil
	e.env["floor"] = math.Floor
	e.env["sin"] = math.Sin
	e.env["cos"] = math.Cos

	// curve(x, x0, y0, x1, y1, ...) performs piecewise-linear interpolation across
	// arbitrary control points. Points must be sorted by x. Clamps below x0 and above xN.
	e.env["curve"] = func(x float64, pts ...float64) float64 {
		n := len(pts)
		if n < 2 {
			return 0
		}
		if n%2 != 0 {
			n--
		}
		if x <= pts[0] {
			return pts[1]
		}
		if x >= pts[n-2] {
			return pts[n-1]
		}
		for i := 0; i < n-2; i += 2 {
			x0, y0 := pts[i], pts[i+1]
			x1, y1 := pts[i+2], pts[i+3]
			if x >= x0 && x <= x1 {
				if x1 == x0 {
					return y0
				}
				t := (x - x0) / (x1 - x0)
				return y0 + t*(y1-y0)
			}
		}
		return pts[n-1]
	}

	// scurve(x, power) provides a smooth S-curve / soft-saturation curve for x in [0, 1].
	e.env["scurve"] = func(x, power float64) float64 {
		x = clamp01(x)
		if power <= 0 {
			power = 1
		}
		if x < 0.5 {
			return 0.5 * math.Pow(2*x, power)
		}
		return 1.0 - 0.5*math.Pow(2*(1-x), power)
	}
}

func (e *Engine) compileRule(pr profile.Rule, idx int) (*rule, error) {
	label := pr.Name
	if label == "" {
		label = fmt.Sprintf("rules[%d]", idx)
	}
	fail := func(err error) (*rule, error) {
		return nil, fmt.Errorf("profile %s: %s: %w", e.p.Name, label, err)
	}

	r := &rule{
		name:     label,
		target:   target(pr.Target),
		priority: pr.Priority,
		params:   map[int]*vm.Program{},
		modulate: pr.Modulate,
		hold:     pr.Hold.Duration(),
	}
	if pr.Effect != "" && r.target != targetSpeak && r.target != targetRadio {
		mode, ok := hid.ParseMode(pr.Effect)
		if !ok {
			return fail(fmt.Errorf("unknown effect %q", pr.Effect))
		}
		r.mode = mode
	}

	if pr.When != "" {
		prog, err := e.compile(pr.When)
		if err != nil {
			return fail(fmt.Errorf("when: %w", err))
		}
		r.when = prog
	}

	switch r.target {
	case targetL2, targetR2:
		for name, src := range pr.Params {
			idx, ok := hid.ParamIndex(r.mode, name)
			if !ok {
				return fail(fmt.Errorf("effect %q has no parameter %q (use p0..p9 for raw access)", r.mode, name))
			}
			prog, err := e.compile(src)
			if err != nil {
				return fail(fmt.Errorf("params.%s: %w", name, err))
			}
			r.params[idx] = prog
		}
	case targetRumbleLeft, targetRumbleRight, targetHapticLeft, targetHapticRight,
		targetPlayerLEDs, targetMuteLED:
		prog, err := e.compile(pr.Value)
		if err != nil {
			return fail(fmt.Errorf("value: %w", err))
		}
		r.value = prog
	case targetLightbar:
		for i, k := range []string{"r", "g", "b"} {
			prog, err := e.compile(pr.Params[k])
			if err != nil {
				return fail(fmt.Errorf("params.%s: %w", k, err))
			}
			r.rgb[i] = prog
		}
	case targetSpeak, targetRadio:
		clip := pr.Clip
		if clip == "" {
			clip = pr.Effect
		}
		if clip == "" {
			clip = pr.Value
		}
		r.clip = clip
		r.cooldown = pr.Cooldown.Duration()
		r.text = pr.Text
	}
	return r, nil
}

func (e *Engine) compile(src string) (*vm.Program, error) {
	if src == "" {
		return nil, fmt.Errorf("empty expression")
	}
	prog, err := expr.Compile(src, expr.Env(e.env))
	if err != nil {
		return nil, err
	}
	// Dry run with zeroed signals: catches type errors that only appear at
	// evaluation time (e.g. string arithmetic).
	if _, err := e.vmr.Run(prog, e.env); err != nil {
		return nil, err
	}
	return prog, nil
}

// Step evaluates all rules for the current time and returns one controller
// frame. It is safe to call at a fixed tick rate; nothing in here blocks.
func (e *Engine) Step(now time.Time) hid.Frame {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = now

	vals, fresh := e.set.Snapshot(now, e.p.TTL())
	e.fresh = fresh
	for _, n := range e.names {
		e.env[n] = float64(0)
	}
	for n, v := range vals {
		if _, ok := e.env[n]; ok {
			e.env[n] = v
		}
	}
	for _, n := range e.names {
		e.cur[n] = e.env[n].(float64)
	}

	var f hid.Frame
	written := make(map[target]bool, 8)
	var speechCandidates []SpeechEvent
	for _, r := range e.rules {
		isActive := e.active(r)
		if r.target == targetSpeak || r.target == targetRadio {
			eligible := false
			if r.cooldown > 0 {
				if r.lastFired.IsZero() || now.Sub(r.lastFired) >= r.cooldown {
					eligible = true
				}
			} else {
				if !r.wasActive {
					eligible = true
				}
			}
			if isActive && eligible {
				speechCandidates = append(speechCandidates, SpeechEvent{
					Name:     r.name,
					Target:   string(r.target),
					Clip:     r.clip,
					Priority: r.priority,
					Text:     r.text,
					At:       now,
				})
				r.lastFired = now
			}
			r.wasActive = isActive
			continue
		}
		if !isActive {
			continue
		}
		r.lastFired = now
		e.apply(r, &f, written)
	}
	if len(speechCandidates) > 0 {
		slices.SortStableFunc(speechCandidates, func(a, b SpeechEvent) int {
			return cmp.Compare(b.Priority, a.Priority)
		})
		e.speechMu.Lock()
		handler := e.speechHandler
		if handler == nil {
			e.speechEvents = append(e.speechEvents, speechCandidates...)
			const maxQueuedSpeechEvents = 64
			if len(e.speechEvents) > maxQueuedSpeechEvents {
				e.speechEvents = e.speechEvents[len(e.speechEvents)-maxQueuedSpeechEvents:]
			}
		}
		e.speechMu.Unlock()

		if handler != nil {
			for _, ev := range speechCandidates {
				handler(ev)
			}
		}
	}
	e.decay(&f, written, now)

	e.prev = e.cur
	e.cur = make(map[string]float64, len(e.names))
	return f
}

func (e *Engine) active(r *rule) bool {
	if r.when == nil {
		r.lastState = RuleStateTrue
		return true
	}
	out, err := e.vmr.Run(r.when, e.env)
	if err != nil {
		e.reportErr(r, err)
		r.lastState = RuleStateError
		return false
	}
	if truthy(out) {
		if r.cooldown > 0 && !r.lastFired.IsZero() && e.now.Sub(r.lastFired) < r.cooldown {
			r.lastState = RuleStateCooldown
			return false
		}
		if r.hold > 0 {
			r.until = e.now.Add(r.hold)
		}
		r.lastState = RuleStateTrue
		return true
	}
	if r.hold > 0 && e.now.Before(r.until) {
		r.lastState = RuleStateHold
		return true
	}
	r.lastState = RuleStateFalse
	return false
}

func (e *Engine) apply(r *rule, f *hid.Frame, written map[target]bool) {
	switch r.target {
	case targetL2, targetR2:
		t, err := e.buildTrigger(r)
		if err != nil {
			e.reportErr(r, err)
			return
		}
		if r.target == targetL2 {
			f.L2 = t
		} else {
			f.R2 = t
		}
		e.lastTrigger[r.target] = t
	case targetRumbleLeft, targetRumbleRight, targetHapticLeft, targetHapticRight:
		v, err := e.eval(r.value)
		if err != nil {
			e.reportErr(r, err)
			return
		}
		n := e.modulateVal(r, v)
		switch r.target {
		case targetRumbleLeft:
			f.RumbleL = n
		case targetRumbleRight:
			f.RumbleR = n
		case targetHapticLeft:
			f.HapticL = n
		case targetHapticRight:
			f.HapticR = n
		}
		e.lastRumble[r.target] = n
	case targetLightbar:
		var rgb [3]uint8
		for i, prog := range r.rgb {
			v, err := e.eval(prog)
			if err != nil {
				e.reportErr(r, err)
				return
			}
			rgb[i] = clamp8(v)
		}
		f.Lightbar = rgb
	case targetPlayerLEDs:
		v, err := e.eval(r.value)
		if err != nil {
			e.reportErr(r, err)
			return
		}
		f.PlayerLEDs = clamp8(v)
	case targetMuteLED:
		v, err := e.eval(r.value)
		if err != nil {
			e.reportErr(r, err)
			return
		}
		f.MuteLED = v >= 0.5
	}
	delete(e.releaseAt, r.target)
	written[r.target] = true
}

func (e *Engine) buildTrigger(r *rule) (hid.Trigger, error) {
	t := hid.Trigger{Mode: r.mode}
	if r.mode == hid.ModeOff {
		return t, nil
	}
	for idx, prog := range r.params {
		v, err := e.eval(prog)
		if err != nil {
			return t, err
		}
		t.Set(idx, v)
	}
	return t, nil
}

// decay fades out effects whose rules stopped applying, so a trigger never
// snaps open when a game exits or a source stalls.
func (e *Engine) decay(f *hid.Frame, written map[target]bool, now time.Time) {
	rel := e.p.ReleaseTime()
	for _, tg := range []target{targetL2, targetR2} {
		if written[tg] {
			continue
		}
		from := e.lastTrigger[tg]
		if from.IsOff() {
			continue
		}
		at, started := e.releaseAt[tg]
		if !started {
			at = now
			e.releaseAt[tg] = now
		}
		if rel <= 0 {
			e.lastTrigger[tg] = hid.Trigger{}
			continue
		}
		frac := 1 - now.Sub(at).Seconds()/rel.Seconds()
		if frac <= 0 {
			e.lastTrigger[tg] = hid.Trigger{}
			delete(e.releaseAt, tg)
			continue
		}
		t := from.Scaled(frac)
		if tg == targetL2 {
			f.L2 = t
		} else {
			f.R2 = t
		}
	}

	for _, tg := range []target{
		targetRumbleLeft, targetRumbleRight, targetHapticLeft, targetHapticRight,
	} {
		if written[tg] {
			continue
		}
		from := e.lastRumble[tg]
		if from == 0 {
			continue
		}
		at, started := e.releaseAt[tg]
		if !started {
			at = now
			e.releaseAt[tg] = now
		}
		if rel <= 0 {
			e.lastRumble[tg] = 0
			continue
		}
		frac := 1 - now.Sub(at).Seconds()/rel.Seconds()
		if frac <= 0 {
			e.lastRumble[tg] = 0
			delete(e.releaseAt, tg)
			continue
		}
		v := uint8(math.Round(float64(from) * frac))
		switch tg {
		case targetRumbleLeft:
			f.RumbleL = v
		case targetRumbleRight:
			f.RumbleR = v
		case targetHapticLeft:
			f.HapticL = v
		case targetHapticRight:
			f.HapticR = v
		}
	}
}

func (e *Engine) modulateVal(r *rule, v float64) uint8 {
	if r.modulate == nil || r.modulate.Hz <= 0 {
		return clamp8(v)
	}
	phase := 2 * math.Pi * r.modulate.Hz * e.now.Sub(e.start).Seconds()
	osc := 0.5 * (1 + math.Sin(phase))
	if r.modulate.Type == "square" {
		if osc >= 0.5 {
			osc = 1
		} else {
			osc = 0
		}
	}
	depth := clamp01(r.modulate.Depth)
	amp := (1 - depth) + depth*osc
	return clamp8(v * amp)
}

func (e *Engine) eval(prog *vm.Program) (float64, error) {
	if prog == nil {
		return 0, nil
	}
	out, err := e.vmr.Run(prog, e.env)
	if err != nil {
		return 0, err
	}
	return toFloat(out)
}

// reportErr logs at most once per second per rule: rules are evaluated at up
// to 250 Hz, so naive logging would flood the terminal.
func (e *Engine) reportErr(r *rule, err error) {
	e.errCount[r.name]++
	if e.now.Sub(e.errLogAt) < time.Second {
		return
	}
	e.errLogAt = e.now
	for name, n := range e.errCount {
		if n > 1 {
			e.log.Printf("rule %q: %v (x%d)", name, err, n)
		} else {
			e.log.Printf("rule %q: %v", name, err)
		}
		delete(e.errCount, name)
	}
}

func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint8:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case nil:
		return 0, nil
	}
	return 0, fmt.Errorf("expression returned %T, expected a number", v)
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		return false
	}
	f, err := toFloat(v)
	return err == nil && f != 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}

// TakeSpeechEvents extracts and clears all speech events accumulated during Step.
func (e *Engine) TakeSpeechEvents() []SpeechEvent {
	e.speechMu.Lock()
	defer e.speechMu.Unlock()
	events := e.speechEvents
	e.speechEvents = nil
	return events
}

// SetSpeechHandler configures an optional immediate callback for speech events.
func (e *Engine) SetSpeechHandler(fn func(SpeechEvent)) {
	e.speechMu.Lock()
	defer e.speechMu.Unlock()
	e.speechHandler = fn
}

// Telemetry returns execution metadata and states of all compiled rules.
func (e *Engine) Telemetry(now time.Time) []RuleTelemetry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]RuleTelemetry, len(e.rules))
	for i, r := range e.rules {
		var untilMS uint32
		if !r.until.IsZero() && now.Before(r.until) {
			untilMS = uint32(r.until.Sub(now).Milliseconds())
		}
		var lastFireMS uint32
		if !r.lastFired.IsZero() {
			d := now.Sub(r.lastFired)
			if d > 0 {
				lastFireMS = uint32(d.Milliseconds())
			}
		}
		out[i] = RuleTelemetry{
			RuleID:     uint16(i),
			Name:       r.name,
			Target:     string(r.target),
			Priority:   int16(r.priority),
			State:      r.lastState,
			LastFireMS: lastFireMS,
			UntilMS:    untilMS,
		}
	}
	return out
}

// Reload re-compiles rules from a new profile and safely transfers timing state
// without losing active timers for matching rules.
func (e *Engine) Reload(p *profile.Profile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("reload: validate profile: %w", err)
	}

	tempEng := &Engine{
		p:     p,
		set:   e.set,
		log:   e.log,
		names: append(append([]string{}, signal.Standard...), p.Extra...),
		prev:  map[string]float64{},
		cur:   map[string]float64{},
		fresh: map[string]bool{},
		start: e.start,
	}
	tempEng.env = make(map[string]any, len(tempEng.names)+10)
	for _, n := range tempEng.names {
		tempEng.env[n] = float64(0)
	}
	tempEng.installHelpers()

	var newRules []*rule
	for i, pr := range p.Rules {
		r, err := tempEng.compileRule(pr, i)
		if err != nil {
			return fmt.Errorf("reload: compile rule: %w", err)
		}
		newRules = append(newRules, r)
	}
	slices.SortStableFunc(newRules, func(a, b *rule) int { return cmp.Compare(a.priority, b.priority) })

	e.mu.Lock()
	defer e.mu.Unlock()

	oldByName := make(map[string]*rule, len(e.rules))
	for _, r := range e.rules {
		oldByName[r.name] = r
	}
	for _, nr := range newRules {
		if old, ok := oldByName[nr.name]; ok {
			nr.until = old.until
			nr.lastFired = old.lastFired
			nr.wasActive = old.wasActive
			nr.lastState = old.lastState
		}
	}

	e.p = p
	e.rules = newRules
	e.names = tempEng.names
	e.env = tempEng.env
	e.installHelpers()
	return nil
}

// Simulate runs an isolated evaluation of the current rules against given signals
// without affecting the active daemon state or physical hardware.
func (e *Engine) Simulate(now time.Time, signals map[string]float64) (hid.Frame, []RuleTelemetry) {
	e.mu.RLock()
	prof := e.p
	e.mu.RUnlock()

	var shapers map[string]signal.Shaper
	if prof != nil && len(prof.Shape) > 0 {
		shapers = make(map[string]signal.Shaper, len(prof.Shape))
		for name, s := range prof.Shape {
			shapers[name] = signal.Shaper{
				Deadzone: s.Deadzone,
				Gamma:    s.Gamma,
				SlewUp:   s.SlewUp,
				SlewDown: s.SlewDown,
			}
		}
	}

	simSet := signal.NewSet(shapers)
	vals32 := make(map[string]float32, len(signals))
	for k, v := range signals {
		vals32[k] = float32(v)
	}
	simSet.Apply(signal.Frame{
		Values: vals32,
		At:     now,
	})

	simEng, err := New(prof, simSet, nil)
	if err != nil {
		return hid.Frame{}, nil
	}
	frame := simEng.Step(now)
	telemetry := simEng.Telemetry(now)
	return frame, telemetry
}
