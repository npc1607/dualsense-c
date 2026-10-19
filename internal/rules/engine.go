// Package rules compiles a profile's declarative rules into an expression
// engine and evaluates them once per output tick.
package rules

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
)

type target string

const (
	targetL2          target = "l2"
	targetR2          target = "r2"
	targetRumbleLeft  target = "rumble_left"
	targetRumbleRight target = "rumble_right"
	targetLightbar    target = "lightbar"
	targetPlayerLEDs  target = "player_leds"
)

type rule struct {
	name     string
	target   target
	mode     hid.Mode
	priority int
	when     *vm.Program
	value    *vm.Program
	params   map[int]*vm.Program
	rgb      [3]*vm.Program
	modulate *profile.Modulate
	hold     time.Duration
	until    time.Time
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
	sort.SliceStable(e.rules, func(i, j int) bool { return e.rules[i].priority < e.rules[j].priority })
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
	if pr.Effect != "" {
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
	case targetRumbleLeft, targetRumbleRight, targetPlayerLEDs:
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
	for _, r := range e.rules {
		if !e.active(r) {
			continue
		}
		e.apply(r, &f, written)
	}
	e.decay(&f, written, now)

	e.prev = e.cur
	e.cur = make(map[string]float64, len(e.names))
	return f
}

func (e *Engine) active(r *rule) bool {
	if r.when == nil {
		return true
	}
	out, err := e.vmr.Run(r.when, e.env)
	if err != nil {
		e.reportErr(r, err)
		return false
	}
	if truthy(out) {
		if r.hold > 0 {
			r.until = e.now.Add(r.hold)
		}
		return true
	}
	if r.hold > 0 && e.now.Before(r.until) {
		return true
	}
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
	case targetRumbleLeft, targetRumbleRight:
		v, err := e.eval(r.value)
		if err != nil {
			e.reportErr(r, err)
			return
		}
		n := e.modulateVal(r, v)
		if r.target == targetRumbleLeft {
			f.RumbleL = n
		} else {
			f.RumbleR = n
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

	for _, tg := range []target{targetRumbleLeft, targetRumbleRight} {
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
		if tg == targetRumbleLeft {
			f.RumbleL = v
		} else {
			f.RumbleR = v
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
