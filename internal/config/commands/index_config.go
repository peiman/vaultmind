package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IndexMetadata defines the metadata for the index command.
var IndexMetadata = config.CommandMetadata{
	Use:   "index",
	Short: "Build or update the vault index",
	Long: `Scan the vault and update the SQLite index.
By default, uses incremental indexing — only re-parses files whose content has changed.
Use --full to force a complete rebuild from scratch.`,
	ConfigPrefix: "app.index",
	FlagOverrides: map[string]string{
		"app.index.vault": "vault",
		"app.index.json":  "json",
		"app.index.full":  "full",
		"app.index.embed": "embed",
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
		{
			Key:          "app.index.full",
			DefaultValue: false,
			Description:  "Force full rebuild instead of incremental index",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
		{
			Key:          "app.index.embed",
			DefaultValue: false,
			Description:  "Compute and store embeddings for note bodies",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(IndexOptions)
}
