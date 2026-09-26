package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// WriteOptions are the settings every command that writes a note shares.
func WriteOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.embed_on_write",
			DefaultValue: true,
			Description: "Embed a note as soon as a command writes it (note create, the frontmatter commands), " +
				"with the model the vault already uses. A changed note loses its embeddings, and without this " +
				"it drops out of semantic search until the next `vaultmind index --embed`. Turn off for bulk " +
				"edits and embed once at the end.",
			Type: "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(WriteOptions)
}
