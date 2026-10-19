package profile

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// builtin loads a profile embedded in the binary.
func builtin(name string) (*Profile, error) {
	raw, err := builtinFS.ReadFile(path.Join("builtin", name+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("profile: builtin %q not found", name)
	}
	var p Profile
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("profile %s: %w", name, err)
	}
	if p.Name == "" {
		p.Name = name
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// BuiltinNames lists embedded profile names.
func BuiltinNames() []string {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), ".yaml"); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Builtin returns all embedded profiles.
func Builtin() ([]*Profile, error) {
	var out []*Profile
	for _, name := range BuiltinNames() {
		p, err := builtin(name)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Resolve finds a profile by name: a path, a file in dir, or a builtin.
func Resolve(nameOrPath, dir string) (*Profile, error) {
	if strings.ContainsAny(nameOrPath, `/\`) || strings.HasSuffix(nameOrPath, ".yaml") {
		return LoadFile(nameOrPath)
	}
	if dir != "" {
		if p, err := LoadFile(fmt.Sprintf("%s/%s.yaml", dir, nameOrPath)); err == nil {
			return p, nil
		}
	}
	return builtin(nameOrPath)
}
