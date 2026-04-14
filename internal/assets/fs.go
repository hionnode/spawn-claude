package assets

import "embed"

//go:embed platform/com.grafana.alloy.plist
//go:embed presets/_base.alloy
var FS embed.FS
