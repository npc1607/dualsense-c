package bridge

import (
	"testing"

	"github.com/npc1607/dualsense-c/internal/signal"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	thr, _ := signal.ID("throttle")
	brk, _ := signal.ID("brake")
	in := []Field{{ID: thr, Value: 0.75}, {ID: brk, Value: 0.25}}

	b, err := Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round trip mismatch: %v", out)
	}
}

func TestDecodeRejectsJunk(t *testing.T) {
	if _, err := Decode([]byte{1, 2, 3}); err != ErrShortPacket {
		t.Errorf("short packet: %v", err)
	}
	bad := make([]byte, HeaderSize)
	if _, err := Decode(bad); err != ErrBadMagic {
		t.Errorf("bad magic: %v", err)
	}
	good, _ := Encode([]Field{{ID: 1, Value: 1}})
	if _, err := Decode(good[:HeaderSize+2]); err != ErrShortPacket {
		t.Error("truncated field block accepted")
	}
	bumped := append([]byte{}, good...)
	bumped[4] = 9 // bump version
	if _, err := Decode(bumped); err == nil {
		t.Error("unsupported version accepted")
	}
}

func TestLayoutOffsetsAndDerivation(t *testing.T) {
	l, ok := Layouts["acc"]
	if !ok {
		t.Fatal("acc layout missing")
	}
	if l.Section != "acpmf_physics" {
		t.Errorf("section = %q", l.Section)
	}
	vals := map[string]float32{
		"slip_fl": 0.4, "slip_fr": -0.9, "slip_rl": 0.2, "slip_rr": 0.1,
	}
	l.Derive(vals)
	if vals["slip"] != 0.9 {
		t.Errorf("derived slip = %v, want 0.9 (max abs)", vals["slip"])
	}
	for _, name := range l.Signals() {
		if _, ok := signal.ID(name); !ok {
			t.Errorf("layout signal %q is not in the vocabulary", name)
		}
	}
}
