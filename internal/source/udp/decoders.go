package udp

import (
	"encoding/binary"
	"log"
	"math"
)

// ---------------------------------------------------------------- Forza -----

// forzaDecoder reads the "Data Out" Dash packet used by Forza Horizon 4/5,
// Forza Motorsport 7 and the 2023 Forza Motorsport.
//
// In the game: Settings -> HUD and Gameplay -> Data Out
//
//	Data Out = ON
//	IP address = this machine
//	Port = 5300
//	Packet format = Dash        <-- Sled (232 B) has no throttle/brake/speed
//
// Layout (little-endian, Dash = 311 bytes; trailing bytes are ignored):
//
//	 0 isRaceOn int32        16 currentEngineRpm f32     8 engineMaxRpm f32
//	20 ax f32               24 ay f32                  28 az f32
//	84 tireSlipRatio[4] f32 (FL,FR,RL,RR)
//	116 wheelOnRumbleStrip[4] i32
//	148 surfaceRumble[4] f32
//	180 tireCombinedSlip[4] f32
//	244 speed f32 (m/s)
//	303 accel u8  304 brake u8  305 clutch u8  306 handbrake u8
//	307 gear u8   308 steer i8
type forzaDecoder struct{ baseDecoder }

func (d *forzaDecoder) Name() string { return "forza" }

func (d *forzaDecoder) Decode(b []byte) (map[string]float32, bool) {
	if !d.accepts(b) {
		return nil, false
	}
	maxRPM := f32(b, 8)
	rpm := f32(b, 16)
	speed := f32(b, 244)
	// Sanity check: a real packet always carries a plausible redline. This is
	// what keeps the autodetector from mis-claiming other games' datagrams.
	if !plausible(maxRPM, 100, 30000) || !plausible(rpm, -100, 30000) ||
		!plausible(speed, -1, 250) {
		return nil, false
	}

	m := map[string]float32{
		"throttle": float32(b[303]) / 255,
		"brake":    float32(b[304]) / 255,
		"clutch":   float32(b[305]) / 255,
		"speed":    speed * 3.6,
		"rpm":      clamp01f(rpm / maxRPM),
		"gear":     float32(b[307]),
		"steer":    float32(int8(b[308])) / 127,
		"accel_g":  f32(b, 28) / 9.81,
		"lat_g":    f32(b, 20) / 9.81,
	}

	var slip, lockup, surface float32
	rumbleStrip := float32(0)
	for i := 0; i < 4; i++ {
		sr := f32(b, 84+4*i)
		combined := f32(b, 180+4*i)
		if a := absf(sr); a > slip {
			slip = a
		}
		if sr < lockup {
			lockup = sr
		}
		if c := absf(combined); c > slip {
			slip = c
		}
		if r := f32(b, 148+4*i); r > surface {
			surface = r
		}
		if int32(binary.LittleEndian.Uint32(b[116+4*i:])) != 0 {
			rumbleStrip = 1
		}
	}
	m["slip"] = clamp01f(slip / 2)
	m["lockup"] = clamp01f(-lockup / 2)
	m["surface"] = clamp01f(surface)
	m["custom0"] = rumbleStrip // rumble strips / kerbs
	return m, true
}

// ------------------------------------------------------------- OutGauge ----

// outgaugeDecoder reads the LFS OutGauge packet, which BeamNG.drive and Live
// For Speed emit (BeamNG: Options -> Other -> OutGauge, port 4444).
//
// Layout (little-endian, 92+ bytes):
//
//	0 time u32   4 car char[4]   8 flags u16   10 gear u8   11 plid u8
//	12 speed f32 (m/s)   16 rpm f32   20 turbo f32   24 engTemp f32
//	28 fuel f32   32 oilPressure f32   36 oilTemp f32
//	40 dashLights u32   44 showLights u32
//	48 throttle f32   52 brake f32   56 clutch f32
type outgaugeDecoder struct {
	baseDecoder
	maxRPM float64
}

func (d *outgaugeDecoder) Name() string { return "outgauge" }

func (d *outgaugeDecoder) Decode(b []byte) (map[string]float32, bool) {
	if !d.accepts(b) {
		return nil, false
	}
	throttle := f32(b, 48)
	brake := f32(b, 52)
	clutch := f32(b, 56)
	speed := f32(b, 12)
	rpm := f32(b, 16)
	if !plausible(throttle, 0, 1.2) || !plausible(brake, 0, 1.2) ||
		!plausible(clutch, 0, 1.2) || !plausible(speed, -1, 250) ||
		!plausible(rpm, 0, 30000) {
		return nil, false
	}
	gear := b[10]
	return map[string]float32{
		"throttle": throttle,
		"brake":    brake,
		"clutch":   clutch,
		"speed":    speed * 3.6,
		"rpm":      clamp01f(rpm / float32(d.maxRPM)),
		"gear":     float32(gear),
		"custom0":  float32(binary.LittleEndian.Uint32(b[40:])), // dash lights bitmask
	}, true
}

// ------------------------------------------------------ Codemasters (DiRT) --

// dirtDecoder handles the Codemasters UDP family used by DiRT Rally 1/2,
// DiRT 4 and EA Sports WRC (they all send to UDP 20777).
//
// The family has changed layout several times, so instead of guessing we ship
// a packet-type dispatcher plus a field table that profiles override. Use
// `ds5ctl sniff -listen :20777` to capture a real packet, then set the offsets
// in the profile's `fields:` block -- no rebuild required.
//
// Each packet starts with a 1-byte packet type; the input packet (type 7)
// carries throttle/brake at the tail and is what these profiles use.
type dirtDecoder struct {
	fields *fieldsDecoder
}

func newDirtDecoder(params map[string]any, logger *log.Logger) (decoder, error) {
	fd, err := newFieldsDecoder(params, logger)
	if err != nil {
		return nil, err
	}
	return &dirtDecoder{fields: fd}, nil
}

func (d *dirtDecoder) Name() string { return "dirt" }

func (d *dirtDecoder) Decode(b []byte) (map[string]float32, bool) {
	if len(b) < 8 {
		return nil, false
	}
	return d.fields.Decode(b)
}

// ------------------------------------------------------------- helpers -----

func f32(b []byte, off int) float32 {
	if off+4 > len(b) || off < 0 {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))
}

func plausible(v float32, lo, hi float64) bool {
	f := float64(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0) && f >= lo && f <= hi
}

func absf(v float32) float32 { return float32(math.Abs(float64(v))) }

func clamp01f(v float32) float32 {
	if v < 0 || v != v {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
