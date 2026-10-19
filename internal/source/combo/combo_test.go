package combo

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

// fakeSource emits one fixed signal at a fixed rate, for exercising the merge.
type fakeSource struct {
	name  string
	key   string
	value float32
	every time.Duration
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Run(ctx context.Context, out chan<- signal.Frame) error {
	t := time.NewTicker(f.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-t.C:
			select {
			case out <- signal.Frame{At: now, Values: map[string]float32{f.key: f.value}}:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func init() {
	source.Register("combotest_a", func(cfg profile.Source, deps source.Deps) (source.Source, error) {
		return &fakeSource{name: "a", key: "l2", value: 0.25, every: 2 * time.Millisecond}, nil
	})
	source.Register("combotest_b", func(cfg profile.Source, deps source.Deps) (source.Source, error) {
		return &fakeSource{name: "b", key: "bass", value: 0.75, every: 2 * time.Millisecond}, nil
	})
}

func TestComboMergesChildSignals(t *testing.T) {
	cfg := profile.Source{Type: "combo", Params: map[string]any{
		"parts": []any{
			map[string]any{"type": "combotest_a"},
			map[string]any{"type": "combotest_b"},
		},
	}}
	src, err := New(cfg, source.Deps{Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := src.Name(), "combo(a+b)"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	out := make(chan signal.Frame, 512)
	go func() { _ = src.Run(ctx, out) }()

	set := signal.NewSet(nil)
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case f := <-out:
			set.Apply(f)
		case <-deadline:
			vals, _ := set.Snapshot(time.Now(), time.Second)
			if vals["l2"] != 0.25 {
				t.Errorf("l2 = %v, want 0.25 (from part a)", vals["l2"])
			}
			if vals["bass"] != 0.75 {
				t.Errorf("bass = %v, want 0.75 (from part b)", vals["bass"])
			}
			return
		}
	}
}

func TestComboRequiresParts(t *testing.T) {
	if _, err := New(profile.Source{Type: "combo", Params: map[string]any{}}, source.Deps{}); err == nil {
		t.Fatal("expected an error when parts is missing")
	}
	if _, err := New(profile.Source{Type: "combo", Params: map[string]any{
		"parts": []any{map[string]any{"type": "no_such_source"}},
	}}, source.Deps{}); err == nil {
		t.Fatal("expected an error for an unknown child source")
	}
}
