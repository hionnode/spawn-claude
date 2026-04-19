package presets

import "github.com/hionnode/spawn-claude/internal/secrets"

// local-debug is the default "log everything to stderr" config that ships
// with PR1/PR2 as _base.alloy. Listed here so `configure --list` names it.
var localDebug = Preset{
	Name:        "local-debug",
	Description: "OTLP receiver on :4317/:4318 → exporter.debug (no vendor).",
	AssetPath:   "presets/_base.alloy",
	// Direct mode is a no-op for this preset: it has no vendor to talk to.
	DirectEnv: nil,
}

const (
	KeySignozEndpoint      = "SIGNOZ_ENDPOINT"
	KeySignozIngestionKey  = "SIGNOZ_INGESTION_KEY"
	KeyGrafanaOTLPEndpoint = "GRAFANA_CLOUD_OTLP_ENDPOINT"
	KeyGrafanaUsername     = "GRAFANA_CLOUD_OTLP_USERNAME"
	KeyGrafanaPassword     = "GRAFANA_CLOUD_OTLP_PASSWORD"
)

// signoz-cloud forwards telemetry to SignOz Cloud's OTLP gRPC ingest.
// Matches the env block in https://signoz.io/docs/claude-code-monitoring/.
var signozCloud = Preset{
	Name:            "signoz-cloud",
	Description:     "Forward OTLP to SignOz Cloud (signoz.io). Requires SIGNOZ_ENDPOINT + SIGNOZ_INGESTION_KEY.",
	AssetPath:       "presets/signoz-cloud.alloy",
	RequiredSecrets: []string{KeySignozEndpoint, KeySignozIngestionKey},
	DirectEnv: func(s map[string]string) ([]string, error) {
		if err := secrets.Require(s, KeySignozEndpoint, KeySignozIngestionKey); err != nil {
			return nil, err
		}
		return []string{
			"CLAUDE_CODE_ENABLE_TELEMETRY=1",
			"OTEL_METRICS_EXPORTER=otlp",
			"OTEL_LOGS_EXPORTER=otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
			"OTEL_EXPORTER_OTLP_ENDPOINT=" + s[KeySignozEndpoint],
			"OTEL_EXPORTER_OTLP_HEADERS=signoz-ingestion-key=" + s[KeySignozIngestionKey],
			"OTEL_METRIC_EXPORT_INTERVAL=10000",
			"OTEL_LOGS_EXPORT_INTERVAL=5000",
			"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative",
		}, nil
	},
}

// grafana-cloud forwards telemetry to Grafana Cloud's OTLP gateway with
// basic auth. Endpoint + credentials are per-stack; find them under
// "Send data → OpenTelemetry (OTLP)" in the Grafana Cloud UI.
var grafanaCloud = Preset{
	Name:            "grafana-cloud",
	Description:     "Forward OTLP to Grafana Cloud. Requires GRAFANA_CLOUD_OTLP_ENDPOINT + _USERNAME + _PASSWORD.",
	AssetPath:       "presets/grafana-cloud.alloy",
	RequiredSecrets: []string{KeyGrafanaOTLPEndpoint, KeyGrafanaUsername, KeyGrafanaPassword},
	DirectEnv: func(s map[string]string) ([]string, error) {
		if err := secrets.Require(s, KeyGrafanaOTLPEndpoint, KeyGrafanaUsername, KeyGrafanaPassword); err != nil {
			return nil, err
		}
		return []string{
			"CLAUDE_CODE_ENABLE_TELEMETRY=1",
			"OTEL_METRICS_EXPORTER=otlp",
			"OTEL_LOGS_EXPORTER=otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
			"OTEL_EXPORTER_OTLP_ENDPOINT=" + s[KeyGrafanaOTLPEndpoint],
			"OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic " + basicAuth(s[KeyGrafanaUsername], s[KeyGrafanaPassword]),
			"OTEL_METRIC_EXPORT_INTERVAL=10000",
			"OTEL_LOGS_EXPORT_INTERVAL=5000",
			"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative",
		}, nil
	},
}
