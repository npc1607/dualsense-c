//go:build linux

package vpad

import "testing"

func TestScalePedal(t *testing.T) {
	cases := []struct {
		in   int32
		want int32
	}{
		{0, -32768},
		{128, 128},
		{255, 32767},
	}
	for _, c := range cases {
		if got := scalePedal(c.in); got != c.want {
			t.Errorf("scalePedal(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestScaleStick(t *testing.T) {
	cases := []struct {
		in   int32
		want int32
	}{
		{0, -32768},
		{128, 0},
		{255, 32767},
	}
	for _, c := range cases {
		if got := scaleStick(c.in); got != c.want {
			t.Errorf("scaleStick(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampByte(t *testing.T) {
	cases := []struct {
		in   int32
		want int32
	}{
		{-10, 0},
		{0, 0},
		{128, 128},
		{255, 255},
		{300, 255},
	}
	for _, c := range cases {
		if got := clampByte(c.in); got != c.want {
			t.Errorf("clampByte(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCombineTriggers(t *testing.T) {
	cases := []struct {
		name string
		l, r int32
		want int32
	}{
		{"idle", 0, 0, 0},
		{"full_throttle", 0, 255, 32767},
		{"full_brake", 255, 0, -32768},
		{"balanced", 128, 128, 0},
		{"half_throttle", 0, 128, 16447},
		{"half_brake", 128, 0, -16447},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := combineTriggers(c.l, c.r)
			// allow +-2 rounding tolerance on intermediate values
			diff := got - c.want
			if diff < -2 || diff > 2 {
				t.Errorf("combineTriggers(%d, %d) = %d, want %d", c.l, c.r, got, c.want)
			}
		})
	}
}
