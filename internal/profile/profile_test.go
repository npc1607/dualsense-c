package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfile_Builtins(t *testing.T) {
	builtins, err := Builtin(DecodeOptions{})
	if err != nil {
		t.Fatalf("Builtin() failed: %v", err)
	}

	foundIsaac := false
	foundSouls := false
	for _, p := range builtins {
		if p.Name == "isaac" {
			foundIsaac = true
		}
		if p.Name == "souls" {
			foundSouls = true
		}
		if err := p.Validate(); err != nil {
			t.Errorf("builtin profile %q failed validation: %v", p.Name, err)
		}
	}

	if !foundIsaac {
		t.Errorf("builtin profile isaac not found")
	}
	if !foundSouls {
		t.Errorf("builtin profile souls not found")
	}
}

func TestProfile_RejectsDuplicateRuleNames(t *testing.T) {
	p := &Profile{
		Name:   "dup",
		Source: Source{Type: "sim"},
		Rules: []Rule{
			{Name: "a", Target: "rumble_left", Value: "1"},
			{Name: "b", Target: "rumble_left", Value: "1"},
			{Name: "c", Target: "rumble_left", Value: "1"},
			{Name: "foo", Target: "rumble_left", Value: "1"},
			{Name: "e", Target: "rumble_left", Value: "1"},
			{Name: "f", Target: "rumble_left", Value: "1"},
			{Name: "g", Target: "rumble_left", Value: "1"},
			{Name: "foo", Target: "rumble_left", Value: "1"},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected duplicate rule names to fail validation")
	}
	msg := err.Error()
	for _, want := range []string{`duplicate rule name "foo"`, "rules[3]", "rules[7]"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestProfile_AllowsAnonymousRules(t *testing.T) {
	p := &Profile{
		Name:   "anon",
		Source: Source{Type: "sim"},
		Rules: []Rule{
			{Target: "lightbar", Params: map[string]string{"r": "1", "g": "2", "b": "3"}},
			{Target: "lightbar", Params: map[string]string{"r": "4", "g": "5", "b": "6"}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("anonymous rules should validate, got: %v", err)
	}
}

func TestProfile_SampleProfilesValidate(t *testing.T) {
	profs, err := Dir("../../profiles", DecodeOptions{})
	if err != nil {
		t.Fatalf("Dir(../../profiles) failed: %v", err)
	}
	if len(profs) == 0 {
		t.Fatal("expected at least one sample profile")
	}
	for _, p := range profs {
		if err := p.Validate(); err != nil {
			t.Errorf("sample profile %q failed validation: %v", p.Name, err)
		}
	}
}

func TestProfile_TargetSpeakAndRadioValidation(t *testing.T) {
	// 1. Valid speak rule with clip
	pValid := &Profile{
		Name:   "valid",
		Source: Source{Type: "sim"},
		Rules: []Rule{
			{
				Name:   "warn",
				Target: "speak",
				Clip:   "low_hp",
			},
			{
				Name:   "radio_alert",
				Target: "radio",
				Effect: "boss_coming", // effect used as clip name
			},
		},
	}
	if err := pValid.Validate(); err != nil {
		t.Fatalf("expected valid speak/radio rules to pass validation: %v", err)
	}

	// 2. Invalid speak rule missing clip, effect, and value
	pInvalid := &Profile{
		Name:   "invalid",
		Source: Source{Type: "sim"},
		Rules: []Rule{
			{
				Name:   "warn",
				Target: "speak",
			},
		},
	}
	if err := pInvalid.Validate(); err == nil {
		t.Fatal("expected speak rule missing clip/effect/value to fail validation")
	}
}

func TestProfile_AimValidation(t *testing.T) {
	fps, err := LoadFile("../../profiles/fps-aim.yaml", DecodeOptions{})
	if err != nil {
		t.Fatalf("LoadFile fps-aim.yaml failed: %v", err)
	}
	aim := fps.EffectiveAim()
	if aim == nil {
		t.Fatal("expected EffectiveAim() to be non-nil for fps-aim.yaml")
	}
	if aim.Curve.Type != "sigmoid" {
		t.Fatalf("expected curve type sigmoid, got %q", aim.Curve.Type)
	}
	if aim.Filter.Type != "one_euro" {
		t.Fatalf("expected filter type one_euro, got %q", aim.Filter.Type)
	}
	if !aim.Gyro.Enabled {
		t.Fatal("expected gyro enabled")
	}
	if !aim.AntiRecoil.Enabled {
		t.Fatal("expected anti_recoil enabled")
	}

	// Invalid curve type
	fps.Aim = &AimConfig{Curve: AimCurveConfig{Type: "invalid_curve"}}
	if err := fps.Validate(); err == nil {
		t.Fatal("expected error for invalid curve type")
	}

	// Invalid filter type
	fps.Aim = &AimConfig{Filter: AimFilterConfig{Type: "invalid_filter"}}
	if err := fps.Validate(); err == nil {
		t.Fatal("expected error for invalid filter type")
	}

	// Invalid gyro trigger
	fps.Aim = &AimConfig{Gyro: AimGyroConfig{Enabled: true, Trigger: "invalid_trigger"}}
	if err := fps.Validate(); err == nil {
		t.Fatal("expected error for invalid gyro trigger")
	}
}

// writeProfile drops a YAML fixture into a temp dir and returns its path.
func writeProfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestStrictDecodeRejectsUnknownKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown top-level key",
			body: "name: strict\nsource:\n  type: sim\nbogus_top: 1\n",
			want: "bogus_top",
		},
		{
			name: "unknown rule key",
			body: `name: strict
source:
  type: sim
rules:
  - name: r
    target: rumble_left
    value: "1"
    bogus_rule: 1
`,
			want: "bogus_rule",
		},
		{
			name: "unknown nested aim key",
			body: `name: strict
source:
  type: sim
aim:
  curve:
    bogus_curve: 1
`,
			want: "bogus_curve",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProfile(t, tc.body)
			if _, err := LoadFile(path, DecodeOptions{}); err == nil {
				t.Fatalf("strict LoadFile accepted unknown key %q", tc.want)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name the unknown key %q", err, tc.want)
			}
		})
	}
}

// TestStrictDecodeAllowsSourceParams documents the deliberate boundary: strict
// mode covers top-level and rule keys, while Source.Params is an inline open
// map that each source adapter validates.
func TestStrictDecodeAllowsSourceParams(t *testing.T) {
	path := writeProfile(t, `name: open-source
source:
  type: sim
  brand_new_key: 42
`)
	p, err := LoadFile(path, DecodeOptions{})
	if err != nil {
		t.Fatalf("unknown source parameter must not fail strict decoding: %v", err)
	}
	if got := fmt.Sprint(p.Source.Params["brand_new_key"]); got != "42" {
		t.Fatalf("Source.Params[brand_new_key] = %q, want 42", got)
	}
}

func TestLenientDecodeKeepsLegacyBehaviour(t *testing.T) {
	path := writeProfile(t, "name: legacy\nsource:\n  type: sim\nbogus_top: 1\n")
	p, err := LoadFile(path, DecodeOptions{Lenient: true})
	if err != nil {
		t.Fatalf("lenient LoadFile rejected an unknown key: %v", err)
	}
	if p.Name != "legacy" {
		t.Fatalf("name = %q, want legacy", p.Name)
	}
}

// TestAllShippedProfilesDecodeStrictly is the regression gate for the escape
// hatch: builtins and samples must not rely on unknown-key tolerance.
func TestAllShippedProfilesDecodeStrictly(t *testing.T) {
	if _, err := Builtin(DecodeOptions{}); err != nil {
		t.Fatalf("strict decoding of builtin profiles failed: %v", err)
	}
	if _, err := Dir("../../profiles", DecodeOptions{}); err != nil {
		t.Fatalf("strict decoding of sample profiles failed: %v", err)
	}
}

// TestResolveDoesNotHideStrictErrors pins the fallback rule: Resolve only falls
// through to a builtin when the file in -profiles does not exist. A profile that
// exists but fails strict decoding must surface the error, otherwise an unknown
// key in the user's own profile silently resolves to the builtin of that name.
func TestResolveDoesNotHideStrictErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fps.yaml")
	if err := os.WriteFile(path, []byte("name: fps\nbogus: 1\nsource: {type: sim}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Resolve("fps", dir); err == nil {
		t.Fatal("Resolve succeeded on a profile with an unknown key, want error")
	} else if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error does not name the unknown key: %v", err)
	}

	// Lenient mode keeps working on the same file.
	if _, err := Resolve("fps", dir, DecodeOptions{Lenient: true}); err != nil {
		t.Fatalf("lenient Resolve failed: %v", err)
	}

	// A missing file still falls through to the builtin.
	if p, err := Resolve("fps", t.TempDir()); err != nil {
		t.Fatalf("builtin fallback failed: %v", err)
	} else if p.Name != "fps" {
		t.Fatalf("fallback profile = %q, want fps", p.Name)
	}
}
