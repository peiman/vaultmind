package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IndexMetadata defines the metadata for the index command.
var IndexMetadata = config.CommandMetadata{
	Use:   "index",
	Short: "Build or rebuild the vault index",
	Long: `Scan the vault, parse all .md files, and populate the SQLite index.
This is a full rebuild — all notes are re-parsed and re-indexed.
The index can always be rebuilt from the vault content.`,
	ConfigPrefix: "app.index",
	FlagOverrides: map[string]string{
		"app.index.vault": "vault",
		"app.index.json":  "json",
	},
}

// IndexOptions returns configuration options for the index command.
func IndexOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.index.vault",
			DefaultValue: ".",
			Description:  "Path to the vault root directory",
			Type:         "string",
			Required:     false,
			Example:      "./my-vault",
		},
		{
			Key:          "app.index.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(IndexOptions)
}
