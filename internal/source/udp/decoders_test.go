package udp

import (
	"encoding/binary"
	"log"
	"math"
	"testing"
)

func f32At(b []byte, off int, v float32) {
	binary.LittleEndian.PutUint32(b[off:], math.Float32bits(v))
}

// forzaDash builds a 311-byte Dash packet with the values a game would send.
func forzaDash(throttle, brake, speedMS, rpm, maxRPM float32) []byte {
	b := make([]byte, 311)
	binary.LittleEndian.PutUint32(b[0:], 1) // isRaceOn
	f32At(b, 8, maxRPM)
	f32At(b, 16, rpm)
	f32At(b, 244, speedMS)
	b[303] = byte(math.Round(float64(throttle) * 255))
	b[304] = byte(math.Round(float64(brake) * 255))
	b[307] = 4 // gear
	return b
}

func TestForzaDecode(t *testing.T) {
	d := &forzaDecoder{baseDecoder{minLen: 311}}
	pkt := forzaDash(1, 0.5, 50, 6000, 8000) // 50 m/s = 180 km/h

	v, ok := d.Decode(pkt)
	if !ok {
		t.Fatal("Decode rejected a valid Dash packet")
	}
	if v["throttle"] != 1 {
		t.Errorf("throttle = %v", v["throttle"])
	}
	if got := v["brake"]; math.Abs(float64(got)-0.5) > 0.01 {
		t.Errorf("brake = %v, want ~0.5", got)
	}
	if got := v["speed"]; math.Abs(float64(got)-180) > 0.5 {
		t.Errorf("speed = %v km/h, want 180", got)
	}
	if got := v["rpm"]; math.Abs(float64(got)-0.75) > 0.001 {
		t.Errorf("rpm = %v, want 0.75", got)
	}
	if v["gear"] != 4 {
		t.Errorf("gear = %v", v["gear"])
	}
}

func TestForzaRejectsWrongShape(t *testing.T) {
	d := &forzaDecoder{baseDecoder{minLen: 311}}
	if _, ok := d.Decode(make([]byte, 311)); ok {
		t.Error("all-zero packet (maxRPM=0) should be rejected")
	}
	if _, ok := d.Decode(make([]byte, 232)); ok {
		t.Error("232-byte Sled packet should be rejected")
	}
	if _, ok := d.Decode([]byte{1, 2, 3}); ok {
		t.Error("short packet should be rejected")
	}
}

func TestOutGaugeDecode(t *testing.T) {
	b := make([]byte, 96)
	b[10] = 3 // gear
	f32At(b, 12, 30)
	f32At(b, 16, 5000)
	f32At(b, 48, 0.8)
	f32At(b, 52, 0.25)
	f32At(b, 56, 0)

	d := &outgaugeDecoder{baseDecoder: baseDecoder{minLen: 92}, maxRPM: 10000}
	v, ok := d.Decode(b)
	if !ok {
		t.Fatal("Decode rejected a valid OutGauge packet")
	}
	if v["gear"] != 3 {
		t.Errorf("gear = %v", v["gear"])
	}
	if got := v["speed"]; math.Abs(float64(got)-108) > 0.5 {
		t.Errorf("speed = %v, want 108 km/h", got)
	}
	if got := v["rpm"]; math.Abs(float64(got)-0.5) > 0.001 {
		t.Errorf("rpm = %v, want 0.5", got)
	}
	if got := v["throttle"]; math.Abs(float64(got)-0.8) > 0.001 {
		t.Errorf("throttle = %v", got)
	}
}

func TestFieldsDecoder(t *testing.T) {
	params := map[string]any{
		"fields": map[string]any{
			"throttle": map[string]any{"off": 4, "fmt": "f32"},
			"brake":    map[string]any{"off": 8, "fmt": "f32"},
			"speed":    map[string]any{"off": 12, "fmt": "f32", "scale": 3.6},
			"gear":     map[string]any{"off": 2, "fmt": "u8", "bias": -1},
		},
		"derived": map[string]any{
			"pedals": "max(throttle, brake)",
		},
	}
	d, err := newFieldsDecoder(params, log.New(discard{}, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, 16)
	pkt[2] = 4 // gear -> 3 after bias
	f32At(pkt, 4, 0.4)
	f32At(pkt, 8, 0.9)
	f32At(pkt, 12, 10)

	v, ok := d.Decode(pkt)
	if !ok {
		t.Fatal("Decode rejected a valid packet")
	}
	if math.Abs(float64(v["speed"])-36) > 0.01 {
		t.Errorf("speed = %v, want 36", v["speed"])
	}
	if v["gear"] != 3 {
		t.Errorf("gear = %v, want 3 (bias applied)", v["gear"])
	}
	if math.Abs(float64(v["pedals"])-0.9) > 0.01 {
		t.Errorf("derived pedals = %v, want 0.9", v["pedals"])
	}
	if _, ok := d.Decode(pkt[:8]); ok {
		t.Error("truncated packet should be rejected")
	}
}

func TestFieldsDecoderRequiresFields(t *testing.T) {
	if _, err := newFieldsDecoder(map[string]any{}, log.New(discard{}, "", 0)); err == nil {
		t.Error("empty fields should be an error")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
