package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteGetMetadata defines the metadata for the note get command.
var NoteGetMetadata = config.CommandMetadata{
	Use:          "get <id-or-path>",
	Short:        "Get a note's full content and metadata",
	Long:         "Return full note content including frontmatter, body, headings, and blocks. Use --frontmatter-only to omit body text.",
	ConfigPrefix: "app.note",
	FlagOverrides: map[string]string{
		"app.note.vault":            "vault",
		"app.note.json":             "json",
		"app.note.frontmatter_only": "frontmatter-only",
	},
}

// NoteOptions returns configuration options for note commands.
func NoteOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.note.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.note.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.note.frontmatter_only", DefaultValue: false, Description: "Omit body, headings, blocks", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteOptions)
}
