package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ResolveMetadata defines the metadata for the resolve command.
var ResolveMetadata = config.CommandMetadata{
	Use:   "resolve <id-or-title-or-alias>",
	Short: "Resolve a reference to a note",
	Long: `Run entity resolution on the given input string.
Returns matched notes with the resolution tier (id, title, alias, normalized).
If ambiguous, returns all candidates.`,
	ConfigPrefix: "app.resolve",
	FlagOverrides: map[string]string{
		"app.resolve.vault": "vault",
		"app.resolve.json":  "json",
	},
}

// ResolveOptions returns configuration options for the resolve command.
func ResolveOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.resolve.vault",
			DefaultValue: ".",
			Description:  "Path to the vault root directory",
			Type:         "string",
		},
		{
			Key:          "app.resolve.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(ResolveOptions)
}
