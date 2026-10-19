// Package audioai runs a small sound-event model (YAMNet) over system audio and
// publishes the recognised surfaces/events as signals.
//
// The engine itself is platform specific (see engine_*.go): the default build is
// pure Go and reports ErrUnsupported, while `-tags onnx` links the ONNX Runtime
// C API (no Python, no cgo in the default build).
package audioai

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ErrUnsupported is returned when the binary was built without the AI engine.
var ErrUnsupported = errors.New("audioai: built without an inference engine (rebuild with -tags onnx)")

// Engine is the platform-specific inference backend.
type Engine interface {
	// Classify runs one window of mono float32 PCM at 16 kHz and returns the
	// per-class scores (AudioSet order, 521 entries for YAMNet).
	Classify(pcm []float32) ([]float32, error)
	Close() error
}

// SignalLabels maps a project signal to the AudioSet display names that feed
// it. Matching is by exact display name from the class map, which keeps the
// mapping stable across model exports (indices move, names do not).
var SignalLabels = map[string][]string{
	"surface_water": {"Water", "Waterfall", "Splash, splatter", "Drip", "Rain"},
	"surface_metal": {"Clang", "Slam", "Bang", "Metal", "Steel", "Hiss"},
	"surface_wood":  {"Wood", "Creak", "Knock", "Tap"},
	"surface_gravel": {"Crunch", "Scratch", "Rub", "Sanding", "Crushing",
		"Walk, footsteps"},
	"audio_gunshot":   {"Gunshot, gunfire", "Machine gun", "Cap gun", "Artillery fire"},
	"audio_explosion": {"Explosion", "Boom", "Eruption", "Smash, crash"},
	"audio_impact":    {"Thump, thud", "Bang", "Slam", "Clang", "Smash, crash"},
}

// Classifier wraps an engine plus the label mapping.
type Classifier struct {
	eng    Engine
	groups map[string][]int // signal -> class indices
	labels []string         // index -> display name
	once   sync.Once
}

// LabelScore is one entry of TopN.
type LabelScore struct {
	Name  string
	Score float32
}

// TopN returns the highest scoring classes, for calibration/debugging.
func (c *Classifier) TopN(scores []float32, n int) []LabelScore {
	type kv struct {
		i int
		v float32
	}
	all := make([]kv, 0, len(scores))
	for i, v := range scores {
		all = append(all, kv{i, v})
	}
	sort.Slice(all, func(a, b int) bool { return all[a].v > all[b].v })
	if n > len(all) {
		n = len(all)
	}
	out := make([]LabelScore, 0, n)
	for _, e := range all[:n] {
		name := "?"
		if e.i < len(c.labels) {
			name = c.labels[e.i]
		}
		out = append(out, LabelScore{Name: name, Score: e.v})
	}
	return out
}

// Open loads the model and class map.
//
// classMapPath may be empty, in which case only the built-in fallback index
// guesses are unavailable and Open fails: the names in the CSV are what make
// the mapping portable, so we require it.
func Open(modelPath, classMapPath string, threads int) (*Classifier, error) {
	labels, err := LoadClassMap(classMapPath)
	if err != nil {
		return nil, err
	}
	eng, err := openEngine(modelPath, threads)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]int, len(SignalLabels))
	for sig, names := range SignalLabels {
		for _, n := range names {
			for i, got := range labels {
				if strings.EqualFold(strings.TrimSpace(got), n) {
					groups[sig] = append(groups[sig], i)
					break
				}
			}
		}
		sort.Ints(groups[sig])
	}
	return &Classifier{eng: eng, groups: groups, labels: labels}, nil
}

// Close releases the engine.
func (c *Classifier) Close() error { return c.eng.Close() }

// RawScores returns the ungrouped class scores (debugging).
func (c *Classifier) RawScores(pcm []float32) ([]float32, error) { return c.eng.Classify(pcm) }

// Signals classifies one window and reduces the 521 scores to our signals.
// Each signal takes the maximum score among its matching classes, so a single
// confident class is enough.
func (c *Classifier) Signals(pcm []float32) (map[string]float32, error) {
	scores, err := c.eng.Classify(pcm)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float32, len(c.groups))
	for sig, idx := range c.groups {
		var best float32
		for _, i := range idx {
			if i < len(scores) && scores[i] > best {
				best = scores[i]
			}
		}
		if best > 0 {
			out[sig] = best
		}
	}
	return out, nil
}

// LoadClassMap reads a yamnet_class_map.csv into display names by index.
func LoadClassMap(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("audioai: class map: %w", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("audioai: class map header: %w", err)
	}
	nameCol := -1
	idxCol := -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))) {
		case "display_name":
			nameCol = i
		case "index":
			idxCol = i
		}
	}
	if nameCol < 0 || idxCol < 0 {
		return nil, errors.New("audioai: class map needs index and display_name columns")
	}

	out := []string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("audioai: class map: %w", err)
		}
		idx, err := strconv.Atoi(strings.TrimSpace(rec[idxCol]))
		if err != nil || idx < 0 {
			continue
		}
		for len(out) <= idx {
			out = append(out, "")
		}
		out[idx] = strings.TrimSpace(rec[nameCol])
	}
	if len(out) == 0 {
		return nil, errors.New("audioai: class map is empty")
	}
	return out, nil
}
