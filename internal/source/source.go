// Package source defines the adapter interface that feeds signals into the
// rules engine. A new game integration is a new adapter (or, for UDP/shared
// memory games, a decoder entry in an existing one).
package source

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
)

// Deps is what a source may need from the host.
type Deps struct {
	Device device.Device // nil in -dry-run
	Logger *log.Logger
}

// Source produces signal frames until ctx is cancelled. Run must return
// promptly once ctx is done (closing the device unblocks a pending Read).
type Source interface {
	Name() string
	Run(ctx context.Context, out chan<- signal.Frame) error
}

// Factory builds a source from its profile entry.
type Factory func(cfg profile.Source, deps Deps) (Source, error)

var registry = map[string]Factory{}

// Register makes a source type available to profiles. Adapters call this from
// init(); the binary blank-imports source/all to pull them in.
func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic("source: duplicate registration for " + name)
	}
	registry[name] = f
}

// Open builds the source described by the profile.
func Open(cfg profile.Source, deps Deps) (Source, error) {
	f, ok := registry[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("source: unknown type %q (available: %v)", cfg.Type, Names())
	}
	return f(cfg, deps)
}

// Names lists registered source types.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
