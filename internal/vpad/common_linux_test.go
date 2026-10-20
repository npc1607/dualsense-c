//go:build linux

package vpad

import "testing"

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
		if got := ScaleStick(c.in); got != c.want {
			t.Errorf("ScaleStick(%d) = %d, want %d", c.in, got, c.want)
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
		if got := ClampByte(c.in); got != c.want {
			t.Errorf("ClampByte(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
