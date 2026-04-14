package assets

import "embed"

//go:embed platform/com.grafana.alloy.plist
//go:embed presets/*.alloy
var FS embed.FS
