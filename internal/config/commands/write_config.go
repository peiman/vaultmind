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
				"it drops out of semantic search until the next 'vaultmind index --embed'. Turn off for bulk " +
				"edits and embed once at the end.",
			Type: "bool",
		},
		{
			Key:          "app.embed_max_rss_mb",
			DefaultValue: 6144,
			Description: "Stop an embedding pass (index --embed, or the embed after a write) once vaultmind's peak " +
				"memory passes this many MiB. Embedding peaks near 3 GB; 0.10.13's re-embed passed 100 GB with " +
				"nothing to stop it. Notes embedded before the stop are kept, so running the command again continues. " +
				"0 turns the check off.",
			Type: "int",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(WriteOptions)
}
