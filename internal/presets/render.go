package presets

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/hionnode/spawn-claude/internal/assets"
	"github.com/hionnode/spawn-claude/internal/secrets"
)

// Render reads the preset's Alloy-config template from the embedded assets
// FS and substitutes required secrets. Placeholders use `{{ .KEY }}` where
// KEY is the exact env var name (e.g. SIGNOZ_INGESTION_KEY).
func Render(p Preset, secretValues map[string]string) ([]byte, error) {
	if err := secrets.Require(secretValues, p.RequiredSecrets...); err != nil {
		return nil, fmt.Errorf("render preset %s: %w", p.Name, err)
	}
	raw, err := assets.FS.ReadFile(p.AssetPath)
	if err != nil {
		return nil, fmt.Errorf("read preset asset %s: %w", p.AssetPath, err)
	}
	tmpl, err := template.New(p.Name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse preset template %s: %w", p.Name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, secretValues); err != nil {
		return nil, fmt.Errorf("execute preset template %s: %w", p.Name, err)
	}
	return buf.Bytes(), nil
}
