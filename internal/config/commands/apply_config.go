package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ApplyMetadata defines metadata for the apply command.
var ApplyMetadata = config.CommandMetadata{
	Use:          "apply",
	Short:        "Execute a plan file",
	Long:         "Parse a JSON plan file and execute its operations in order. Supports --dry-run, --diff, and --commit.",
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
