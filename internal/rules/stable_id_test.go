package rules

import (
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
)

func TestStableRuleID(t *testing.T) {
	// Same name always maps to the same id, regardless of call site.
	if a, b := StableRuleID("l2-throttle"), StableRuleID("l2-throttle"); a != b {
		t.Fatalf("same name produced different ids: %d != %d", a, b)
	}
	// Empty names are deterministic too (md5 of the empty string, LE prefix).
	if a, b := StableRuleID(""), StableRuleID(""); a != b {
		t.Fatalf("empty name is not deterministic: %d != %d", a, b)
	}
	if StableRuleID("") != 0x1dd4 {
		t.Fatalf("empty name id = %#04x, want 0x1dd4", StableRuleID(""))
	}
	// Different names must not share an id.
	names := []string{"l2-throttle", "r2-brake", "lightbar", "recoil", "L2-throttle"}
	seen := make(map[uint16]string, len(names))
	for _, n := range names {
		id := StableRuleID(n)
		if prev, ok := seen[id]; ok {
			t.Fatalf("id %d collision between %q and %q", id, prev, n)
		}
		seen[id] = n
	}
	// Pin the algorithm so an accidental change to the hash is caught.
	if got := StableRuleID("l2"); got != 0xc2be {
		t.Fatalf("StableRuleID(\"l2\") = %#04x, want 0xc2be (md5(\"l2\")[0:2] LE)", got)
	}
}

func TestStableRuleIDBuiltinProfilesNoCollision(t *testing.T) {
	profs, err := profile.Builtin()
	if err != nil {
		t.Fatalf("profile.Builtin: %v", err)
	}

	total := 0
	// A rule id must be unique inside the profile that is on the wire, and
	// identical names must map to identical ids across profiles.
	global := make(map[uint16]string)
	for _, p := range profs {
		perProfile := make(map[uint16]string)
		for _, r := range p.Rules {
			total++
			if r.Name == "" {
				t.Fatalf("profile %s: builtin rule without a name", p.Name)
			}
			id := StableRuleID(r.Name)
			if prev, ok := perProfile[id]; ok {
				t.Errorf("profile %s: rules %q and %q share rule_id %d", p.Name, prev, r.Name, id)
			}
			perProfile[id] = r.Name
			if prev, ok := global[id]; ok && prev != r.Name {
				t.Errorf("rule_id %d collision between %q and %q", id, prev, r.Name)
			}
			global[id] = r.Name
		}
	}
	if total < 64 {
		t.Fatalf("expected at least 64 builtin rules to exercise the hash, got %d", total)
	}
}

func TestEngineTelemetryRuleIDsStableAcrossReload(t *testing.T) {
	rumble := func(name, value string, priority int) profile.Rule {
		return profile.Rule{Name: name, Target: "rumble_left", Value: value, Priority: priority}
	}
	// The two profiles list the same rules in different source order and with
	// different priorities, so the compiled slices reorder between reloads.
	pBefore := &profile.Profile{
		Name:   "before",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{
			rumble("alpha", "10", 20),
			rumble("beta", "20", 10),
		},
	}
	pAfter := &profile.Profile{
		Name:   "after",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{
			rumble("gamma", "30", 5),
			rumble("alpha", "10", 15),
			rumble("beta", "20", 10),
		},
	}

	e, err := New(pBefore, signal.NewSet(nil), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Now()
	idsBefore := make(map[string]uint16)
	for _, rt := range e.Telemetry(now) {
		idsBefore[rt.Name] = rt.RuleID
	}
	if len(idsBefore) != 2 {
		t.Fatalf("expected 2 rules before reload, got %d", len(idsBefore))
	}

	if err := e.Reload(pAfter); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	telem := e.Telemetry(now)
	idsAfter := make(map[string]uint16)
	for _, rt := range telem {
		idsAfter[rt.Name] = rt.RuleID
	}
	for _, name := range []string{"alpha", "beta"} {
		if idsBefore[name] != idsAfter[name] {
			t.Errorf("rule %q changed id across reload: %d -> %d", name, idsBefore[name], idsAfter[name])
		}
		if idsAfter[name] != StableRuleID(name) {
			t.Errorf("rule %q id = %d, want StableRuleID %d", name, idsAfter[name], StableRuleID(name))
		}
	}
	if _, ok := idsAfter["gamma"]; !ok {
		t.Fatalf("expected gamma after reload, got %v", idsAfter)
	}

	// Slice order is still priority ascending, independent of the stable ids.
	for i := 1; i < len(telem); i++ {
		if telem[i-1].Priority > telem[i].Priority {
			t.Fatalf("telemetry not sorted by priority: %+v", telem)
		}
	}

	// rules.simulate builds its telemetry through an isolated engine; it must
	// report the same stable ids as the live engine.
	_, simTelem := e.Simulate(now, map[string]float64{"throttle": 1})
	for _, rt := range simTelem {
		if idsAfter[rt.Name] != rt.RuleID {
			t.Errorf("simulated rule %q id = %d, live id = %d", rt.Name, rt.RuleID, idsAfter[rt.Name])
		}
	}
}
