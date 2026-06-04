package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// LintFixLinksMetadata defines metadata for the lint fix-links subcommand.
var LintFixLinksMetadata = config.CommandMetadata{
	Use:          "fix-links",
	Short:        "Rewrite wikilinks to Obsidian-compatible format",
	Long:         "Detect [[Title]] wikilinks that won't resolve in Obsidian and rewrite to [[filename|Title]].\nDefault is dry-run (preview). Use --fix to apply changes.",
	ConfigPrefix: "app.lintfixlinks",
	FlagOverrides: map[string]string{
		"app.lintfixlinks.vault": "vault",
		"app.lintfixlinks.json":  "json",
		"app.lintfixlinks.fix":   "fix",
	},
}

// LintFixLinksOptions returns configuration options for the lint fix-links command.
func LintFixLinksOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.lintfixlinks.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.lintfixlinks.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.lintfixlinks.fix", DefaultValue: false, Description: "Apply fixes (default is dry-run)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(LintFixLinksOptions)
}
