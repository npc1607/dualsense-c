package udp

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// fieldsDecoder extracts signals from a fixed packet layout declared in the
// profile. This is the escape hatch that makes a brand-new game a YAML edit:
//
//	source:
//	  type: udp
//	  listen: "0.0.0.0:20777"
//	  decoder: fields
//	  fields:
//	    throttle: { off: 116, fmt: f32 }
//	    brake:    { off: 120, fmt: f32 }
//	    speed:    { off: 8,   fmt: f32, scale: 3.6 }
//	    gear:     { off: 6,   fmt: u8 }
//	  derived:
//	    slip: "max(abs(slip_fl), abs(slip_fr))"
//
// `fmt` is one of u8, s8, u16, s16, u32, s32, f32. `scale` defaults to 1 and
// `bias` to 0, so value = raw*scale + bias.
type fieldSpec struct {
	name  string
	off   int
	kind  string
	scale float64
	bias  float64
	size  int
}

type derived struct {
	name string
	prog *vm.Program
}

type fieldsDecoder struct {
	specs   []fieldSpec
	derived []derived
	minLen  int
	env     map[string]any
	vm      vm.VM
	logger  *log.Logger
}

func newFieldsDecoder(params map[string]any, logger *log.Logger) (*fieldsDecoder, error) {
	if logger == nil {
		logger = log.Default()
	}
	raw := asMap(params["fields"])
	d := &fieldsDecoder{env: map[string]any{}, logger: logger}
	for name, v := range raw {
		spec, err := parseField(name, asMap(v))
		if err != nil {
			return nil, err
		}
		d.specs = append(d.specs, spec)
		d.env[name] = float64(0)
		if end := spec.off + spec.size; end > d.minLen {
			d.minLen = end
		}
	}
	for name, v := range asMap(params["derived"]) {
		src, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("udp fields: derived.%s must be an expression string", name)
		}
		prog, err := expr.Compile(src, expr.Env(d.env))
		if err != nil {
			return nil, fmt.Errorf("udp fields: derived.%s: %w", name, err)
		}
		d.derived = append(d.derived, derived{name: name, prog: prog})
		d.env[name] = float64(0)
	}
	if len(d.specs) == 0 {
		return nil, fmt.Errorf("udp fields: no fields configured (see the fields: block in the profile docs)")
	}
	return d, nil
}

func parseField(name string, m map[string]any) (fieldSpec, error) {
	spec := fieldSpec{name: name, scale: 1, kind: asString(m["fmt"])}
	spec.off = int(asFloat(m["off"]))
	if v, ok := m["scale"]; ok {
		spec.scale = asFloat(v)
	}
	spec.bias = asFloat(m["bias"])
	switch spec.kind {
	case "u8", "s8":
		spec.size = 1
	case "u16", "s16":
		spec.size = 2
	case "u32", "s32", "f32":
		spec.size = 4
	case "":
		return spec, fmt.Errorf("udp fields: %s.fmt is required (u8, s8, u16, s16, u32, s32, f32)", name)
	default:
		return spec, fmt.Errorf("udp fields: %s has unknown fmt %q", name, spec.kind)
	}
	if spec.off < 0 {
		return spec, fmt.Errorf("udp fields: %s has negative offset", name)
	}
	return spec, nil
}

func (d *fieldsDecoder) Name() string { return "fields" }

func (d *fieldsDecoder) Decode(b []byte) (map[string]float32, bool) {
	if len(b) < d.minLen {
		return nil, false
	}
	for _, s := range d.specs {
		v := s.scale*readRaw(b, s) + s.bias
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		d.env[s.name] = v
	}
	for _, dv := range d.derived {
		out, err := d.vm.Run(dv.prog, d.env)
		if err != nil {
			d.logger.Printf("udp fields: derived.%s: %v", dv.name, err)
			continue
		}
		if f, ok := toF(out); ok {
			d.env[dv.name] = f
		}
	}
	out := make(map[string]float32, len(d.env))
	for k, v := range d.env {
		if f, ok := toF(v); ok {
			out[k] = float32(f)
		}
	}
	return out, true
}

func readRaw(b []byte, s fieldSpec) float64 {
	switch s.kind {
	case "u8":
		return float64(b[s.off])
	case "s8":
		return float64(int8(b[s.off]))
	case "u16":
		return float64(binary.LittleEndian.Uint16(b[s.off:]))
	case "s16":
		return float64(int16(binary.LittleEndian.Uint16(b[s.off:])))
	case "u32":
		return float64(binary.LittleEndian.Uint32(b[s.off:]))
	case "s32":
		return float64(int32(binary.LittleEndian.Uint32(b[s.off:])))
	case "f32":
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[s.off:])))
	}
	return 0
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func asMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			if s, ok := k.(string); ok {
				out[s] = val
			}
		}
		return out
	}
	return nil
}

func toF(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}
