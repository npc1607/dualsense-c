package profile

import (
	"testing"
)

func TestProfile_Builtins(t *testing.T) {
	builtins, err := Builtin()
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
