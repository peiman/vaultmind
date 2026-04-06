package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DataviewRenderMetadata defines metadata for the dataview render command.
var DataviewRenderMetadata = config.CommandMetadata{
	Use:   "render <note-id-or-path>",
	Short: "Render a generated region",
	Long: `Replace content between VAULTMIND:GENERATED markers with section template content.

Arguments:
  note-id-or-path   Note ID, title, or file path (resolved via entity resolution)

Markers have the format: <!-- VAULTMIND:GENERATED:{key}:START/END -->
Section templates are loaded from .vaultmind/sections/{type}/{key}.md.
Use --section-key to render a specific section (default: all sections).
Use --force to override checksum mismatch (hand-edited content).`,
	ConfigPrefix: "app.dataviewrender",
	FlagOverrides: map[string]string{
		"app.dataviewrender.vault":       "vault",
		"app.dataviewrender.json":        "json",
		"app.dataviewrender.dry_run":     "dry-run",
		"app.dataviewrender.diff":        "diff",
		"app.dataviewrender.commit":      "commit",
		"app.dataviewrender.force":       "force",
		"app.dataviewrender.section_key": "section-key",
	},
}

// DataviewLintMetadata defines metadata for the dataview lint command.
var DataviewLintMetadata = config.CommandMetadata{
	Use:          "lint",
	Short:        "Validate generated region markers",
	Long:         "Check all notes for malformed or duplicated VAULTMIND:GENERATED markers.",
	ConfigPrefix: "app.dataviewlint",
	FlagOverrides: map[string]string{
		"app.dataviewlint.vault": "vault",
		"app.dataviewlint.json":  "json",
	},
}

// DataviewOptions returns config options for dataview commands.
func DataviewOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.dataviewrender.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.dataviewrender.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.dataviewrender.dry_run", DefaultValue: false, Description: "Preview without writing", Type: "bool"},
		{Key: "app.dataviewrender.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.dataviewrender.commit", DefaultValue: false, Description: "Stage and commit", Type: "bool"},
		{Key: "app.dataviewrender.force", DefaultValue: false, Description: "Override checksum mismatch", Type: "bool"},
		{Key: "app.dataviewrender.section_key", DefaultValue: "", Description: "Section key to render", Type: "string"},
		{Key: "app.dataviewlint.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.dataviewlint.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DataviewOptions)
}
