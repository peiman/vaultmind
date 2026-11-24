package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// PersonaHealthMetadata defines `persona health`.
var PersonaHealthMetadata = config.CommandMetadata{
	Use:   "health",
	Short: "Report whether keeper identity reached the current context",
	Long: `Measure the note text that arrived after the newest transcript
compaction boundary. Identity and arc-body coverage are measured in normalized
body bytes, excluding frontmatter. Principle counts remain a separate signal;
an optional arc-body threshold produces WARN only. A failed keeper makes the
command exit non-zero.

With --keepers, pass a YAML list of {name, transcripts, vault, anchor?} entries.
Without it, pass exactly one --transcripts/--vault pair. The automatic identity
anchor is a note of type identity with id identity-who-i-am or identity-who-am-i.
The single-keeper label defaults to the basename of --transcripts, preserving
any leading path-derived prefix literally; --name overrides it.`,
	ConfigPrefix: "app.persona.health",
	FlagOverrides: map[string]string{
		"app.persona.health.name":             "name",
		"app.persona.health.transcripts":      "transcripts",
		"app.persona.health.vault":            "vault",
		"app.persona.health.anchor":           "anchor",
		"app.persona.health.keepers":          "keepers",
		"app.persona.health.json":             "json",
		"app.persona.health.min_coverage":     "min-coverage",
		"app.persona.health.min_arc_coverage": "min-arc-coverage",
	},
}

// PersonaHealthOptions returns configuration options for `persona health`.
func PersonaHealthOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.persona.health.name", DefaultValue: "", Description: "Single-keeper label (default: basename of --transcripts)", Type: "string"},
		{Key: "app.persona.health.transcripts", DefaultValue: "", Description: "Transcript project directory", Type: "string"},
		{Key: "app.persona.health.vault", DefaultValue: "", Description: "Identity vault path", Type: "string"},
		{Key: "app.persona.health.anchor", DefaultValue: "", Description: "Identity anchor note id", Type: "string"},
		{Key: "app.persona.health.keepers", DefaultValue: "", Description: "YAML file containing keeper definitions", Type: "string"},
		{Key: "app.persona.health.json", DefaultValue: false, Description: "Output JSON", Type: "bool"},
		{Key: "app.persona.health.min_coverage", DefaultValue: 0.9, Description: "Minimum identity coverage fraction", Type: "float"},
		{Key: "app.persona.health.min_arc_coverage", DefaultValue: 0.0, Description: "Minimum total arc-body coverage fraction (WARN only)", Type: "float"},
	}
}

func init() {
	config.RegisterOptionsProvider(PersonaHealthOptions)
}
