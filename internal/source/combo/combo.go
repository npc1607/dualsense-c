// Package combo runs several sources at once and merges their frames.
//
// A profile can only name one source, but some games need two kinds of input at
// the same time: a shooter wants the controller's own trigger travel (for the
// aim wall and recoil) *and* the audio envelope (so gunshots produce haptics
// even though the game exposes no telemetry).
//
// Merging is safe because signal.Set tracks every signal independently, so a
// frame that only carries `bass` does not clear `l2` from another source.
package combo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func init() { source.Register("combo", New) }

// Source fans out Run to its children.
type Source struct {
	children []source.Source
	name     string
}

// New builds the children from the `parts` list.
//
//	source:
//	  type: combo
//	  parts:
//	    - {type: trigger, rate_hz: 125}
//	    - {type: audio, target: auto, gain: 3.0}
func New(cfg profile.Source, deps source.Deps) (source.Source, error) {
	raw, ok := cfg.Params["parts"]
	if !ok {
		return nil, errors.New("combo source: `parts` is required (a list of source blocks)")
	}
	list, ok := asList(raw)
	if !ok || len(list) == 0 {
		return nil, errors.New("combo source: `parts` must be a non-empty list")
	}

	s := &Source{}
	var names []string
	for i, item := range list {
		block, ok := asMap(item)
		if !ok {
			return nil, fmt.Errorf("combo source: parts[%d] is not a mapping", i)
		}
		typ, _ := block["type"].(string)
		if typ == "" {
			return nil, fmt.Errorf("combo source: parts[%d] has no `type`", i)
		}
		params := make(map[string]any, len(block))
		for k, v := range block {
			if k != "type" {
				params[k] = v
			}
		}
		child, err := source.Open(profile.Source{Type: typ, Params: params}, deps)
		if err != nil {
			// In -dry-run there is no controller, so a device-bound part (trigger)
			// cannot start. Dropping it keeps the rest of the profile testable
			// without hardware; a real run still fails loudly.
			if deps.Device == nil {
				if deps.Logger != nil {
					deps.Logger.Printf("combo source: parts[%d] (%s) skipped: %v", i, typ, err)
				}
				continue
			}
			return nil, fmt.Errorf("combo source: parts[%d] (%s): %w", i, typ, err)
		}
		s.children = append(s.children, child)
		names = append(names, child.Name())
	}
	if len(s.children) == 0 {
		return nil, errors.New("combo source: no usable parts")
	}
	// Keep the names for Name(); a profile with several parts shows them all.
	s.name = "combo(" + strings.Join(names, "+") + ")"
	return s, nil
}

func (s *Source) Name() string { return s.name }

// Run starts every child and forwards all of their frames until ctx is done.
// The first fatal error cancels the rest so a broken part cannot silently
// starve the profile of half its signals.
func (s *Source) Run(ctx context.Context, out chan<- signal.Frame) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	merged := make(chan signal.Frame, 256)
	errs := make(chan error, len(s.children))
	for _, child := range s.children {
		go func(c source.Source) {
			if err := c.Run(ctx, merged); err != nil && ctx.Err() == nil {
				errs <- fmt.Errorf("combo source: %s: %w", c.Name(), err)
				return
			}
			errs <- nil
		}(child)
	}

	done := len(s.children)
	var firstErr error
	for {
		select {
		case <-ctx.Done():
			return firstErr
		case f := <-merged:
			select {
			case out <- f:
			case <-ctx.Done():
				return firstErr
			}
		case err := <-errs:
			done--
			if err != nil && firstErr == nil {
				firstErr = err
				cancel()
			}
			if done == 0 {
				return firstErr
			}
		}
	}
}

// asList normalises a YAML sequence decoded into `any`.
func asList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	}
	return nil, false
}

// asMap normalises a YAML mapping decoded into `any`. yaml.v3 gives
// map[string]any, but accept map[any]any too so hand-built configs work.
func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	}
	return nil, false
}
