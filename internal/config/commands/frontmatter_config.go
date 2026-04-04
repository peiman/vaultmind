package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// FrontmatterValidateMetadata defines the metadata for the frontmatter validate command.
var FrontmatterValidateMetadata = config.CommandMetadata{
	Use:          "validate",
	Short:        "Validate frontmatter against type registry",
	Long:         "Check all domain notes for missing required fields, invalid statuses, unknown types, and broken references.",
	ConfigPrefix: "app.frontmatter",
	FlagOverrides: map[string]string{
		"app.frontmatter.vault": "vault",
		"app.frontmatter.json":  "json",
	},
}

// FrontmatterOptions returns configuration options for frontmatter commands.
func FrontmatterOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.frontmatter.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatter.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(FrontmatterOptions)
}
