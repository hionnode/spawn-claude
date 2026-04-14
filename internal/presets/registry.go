package presets

import (
	"fmt"
	"sort"
)

// DirectEnvFn builds the OTEL env var block for `spawn-claude run --direct`
// using this preset. It receives the loaded secrets map and returns the
// KEY=VALUE pairs that should be exported before exec'ing claude. Nil means
// the preset is collector-only and cannot be used in direct mode.
type DirectEnvFn func(secrets map[string]string) ([]string, error)

type Preset struct {
	Name            string
	Description     string
	AssetPath       string // path inside the embedded assets FS, e.g. "presets/signoz-cloud.alloy"
	RequiredSecrets []string
	DirectEnv       DirectEnvFn
}

var registry = map[string]Preset{
	"local-debug":   localDebug,
	"signoz-cloud":  signozCloud,
	"grafana-cloud": grafanaCloud,
}

// Get looks up a preset by name.
func Get(name string) (Preset, error) {
	p, ok := registry[name]
	if !ok {
		return Preset{}, fmt.Errorf("unknown preset %q (see `spawn-claude collector configure --list`)", name)
	}
	return p, nil
}

// All returns every registered preset, sorted by name for stable output.
func All() []Preset {
	out := make([]Preset, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
