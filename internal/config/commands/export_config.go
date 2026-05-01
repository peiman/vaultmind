package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ExportMetadata defines the metadata for the export command — produces
// a sanitized JSONL snapshot of the experiment DB for sharing with the
// VaultMind team. Tier-aware: Anonymous strips note_ids, paths, and
// queries per the contract documented in internal/experiment/telemetry.go;
// Full preserves everything.
var ExportMetadata = config.CommandMetadata{
	Use:   "export",
	Short: "Export experiment data as a sanitized JSONL snapshot",
	Long: `Export the experiment DB (sessions, events, outcomes) as a JSONL
stream that you can share with the VaultMind team. The output is sanitized
according to your configured telemetry tier:

  anonymous  Strips vault paths, query text, and result note_ids/paths.
             Aggregate shape (ranks, scores, counts, timestamps, variant
             names) is preserved.
  full       Preserves everything. Use only if you've explicitly opted
             into Full data sharing on first run.
  off        Refused — the user opted out, so producing a file would
             imply we collected data we shouldn't have.

OUTPUT SHAPE

A JSONL stream — one JSON object per line:

  {"kind":"manifest","tier":"anonymous","exported_at":"...",...}
  {"kind":"session","session_id":"...","started_at":"...","ended_at":"..."}
  {"kind":"event","event_id":"...","event_type":"ask","data":{...}}
  {"kind":"outcome","outcome_id":"...","rank":1,"variant":"primary",...}

EXAMPLES

  vaultmind export
      Stream JSONL to stdout. Pipe to gzip or redirect:
        vaultmind export | gzip > vm-export-$(date +%Y%m%d).jsonl.gz

  vaultmind export --output ./vm-export.jsonl
      Write to a file directly.

  vaultmind export --tier full
      Override the configured tier (requires opt-in record).

PRIVACY

The Anonymous tier's stripping contract is tested in
internal/experiment/export_test.go. If the tests change, the contract
documented in telemetry.go must change too.`,
	ConfigPrefix: "app.export",
	FlagOverrides: map[string]string{
		"app.export.output":  "output",
		"app.export.tier":    "tier",
		"app.export.rollup":  "rollup",
		"app.export.vault":   "vault",
		"app.export.preview": "preview",
	},
}

// ExportOptions returns configuration options for the export command.
func ExportOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.export.output", DefaultValue: "", Description: "Output file path (empty = stdout)", Type: "string"},
		{Key: "app.export.tier", DefaultValue: "", Description: "Override telemetry tier (off|anonymous|full); empty = use experiments.telemetry from config", Type: "string"},
		{Key: "app.export.rollup", DefaultValue: false, Description: "Emit a federated-aggregator-shaped rollup (vault fingerprint + features + variant stats) instead of raw events", Type: "bool"},
		{Key: "app.export.vault", DefaultValue: ".", Description: "Vault path (required when --rollup is set; reads index DB and fingerprint)", Type: "string"},
		{Key: "app.export.preview", DefaultValue: false, Description: "Print a human-readable summary instead of writing the JSON payload (useful for auditing before sharing)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ExportOptions)
}
