package bridge

import "math"

// Kind is how a value is stored in the mapped shared memory.
type Kind uint8

const (
	// KindF32 is a 32-bit float.
	KindF32 Kind = iota
	// KindI32 is a 32-bit signed integer.
	KindI32
)

// FieldSpec is one value to read out of the mapped section.
type FieldSpec struct {
	Signal string // standard signal name, e.g. "throttle"
	Off    int    // byte offset inside the section
	Kind   Kind
}

// MaxAbsGroup derives a signal from the maximum absolute value of a set of
// source signals (used for `slip` = worst wheel slip).
type MaxAbsGroup struct {
	Target  string
	Sources []string
}

// Layout describes one game's shared-memory section.
type Layout struct {
	Name    string
	Section string // Windows named section, e.g. "acpmf_physics"
	Fields  []FieldSpec
	MaxAbs  []MaxAbsGroup
}

// Layouts maps a profile's `layout:` value to a layout.
//
// AC, ACC and Assetto Corsa Rally all publish the same `acpmf_physics`
// structure (ACC's is a superset of AC's, and every field below sits at an
// identical offset in both). ACR was validated against the real game with these
// exact offsets by the acr-dualsense project; see cmd/ds5bridge for the
// provenance note.
var Layouts = map[string]Layout{
	"ac":  assettoCorsa(),
	"acc": assettoCorsa(),
	"acr": assettoCorsa(),
}

func assettoCorsa() Layout {
	return Layout{
		Name:    "ac",
		Section: "acpmf_physics",
		Fields: []FieldSpec{
			{Signal: "throttle", Off: 4, Kind: KindF32}, // gas
			{Signal: "brake", Off: 8, Kind: KindF32},    // brake
			{Signal: "gear", Off: 16, Kind: KindI32},    // gear
			{Signal: "speed", Off: 28, Kind: KindF32},   // speedKmh
			{Signal: "slip_fl", Off: 56, Kind: KindF32}, // wheelSlip[0]
			{Signal: "slip_fr", Off: 60, Kind: KindF32}, // wheelSlip[1]
			{Signal: "slip_rl", Off: 64, Kind: KindF32}, // wheelSlip[2]
			{Signal: "slip_rr", Off: 68, Kind: KindF32}, // wheelSlip[3]
			{Signal: "tc", Off: 204, Kind: KindF32},     // tc
			{Signal: "abs", Off: 252, Kind: KindF32},    // abs
			{Signal: "custom0", Off: 20, Kind: KindI32}, // raw engine rpm
		},
		MaxAbs: []MaxAbsGroup{
			{Target: "slip", Sources: []string{"slip_fl", "slip_fr", "slip_rl", "slip_rr"}},
		},
	}
}

// Signals lists every signal name this layout produces, including derived ones.
func (l Layout) Signals() []string {
	out := make([]string, 0, len(l.Fields)+len(l.MaxAbs))
	for _, f := range l.Fields {
		out = append(out, f.Signal)
	}
	for _, g := range l.MaxAbs {
		out = append(out, g.Target)
	}
	return out
}

// Derive fills in MaxAbs groups from already-read values.
func (l Layout) Derive(vals map[string]float32) {
	for _, g := range l.MaxAbs {
		var max float32
		for _, src := range g.Sources {
			if a := float32(math.Abs(float64(vals[src]))); a > max {
				max = a
			}
		}
		vals[g.Target] = max
	}
}
