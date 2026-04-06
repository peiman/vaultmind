package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ApplyMetadata defines metadata for the apply command.
var ApplyMetadata = config.CommandMetadata{
	Use:   "apply <plan-file | ->",
	Short: "Execute a plan file",
	Long: `Parse a JSON plan file and execute its operations in order.

Arguments:
  plan-file   Path to a JSON plan file, or "-" to read from stdin

The plan file must contain: {"version": 1, "description": "...", "operations": [...]}
Supported operations: frontmatter_set, frontmatter_unset, frontmatter_merge,
generated_region_render, note_create.

Use --dry-run to preview without writing. Use --diff to see unified diffs.
Use --commit to stage and commit all changes in a single git commit.

With --json, returns an envelope with status and per-operation results.`,
	ConfigPrefix: "app.apply",
	FlagOverrides: map[string]string{
		"app.apply.vault":   "vault",
		"app.apply.json":    "json",
		"app.apply.dry_run": "dry-run",
		"app.apply.diff":    "diff",
		"app.apply.commit":  "commit",
	},
}

// ApplyOptions returns config options for the apply command.
func ApplyOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.apply.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.apply.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.apply.dry_run", DefaultValue: false, Description: "Preview without executing", Type: "bool"},
		{Key: "app.apply.diff", DefaultValue: false, Description: "Show unified diffs", Type: "bool"},
		{Key: "app.apply.commit", DefaultValue: false, Description: "Stage and commit all changes", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ApplyOptions)
}
