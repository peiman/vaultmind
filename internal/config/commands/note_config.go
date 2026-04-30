package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteGetMetadata defines the metadata for the note get command.
var NoteGetMetadata = config.CommandMetadata{
	Use:   "get <id-or-path>",
	Short: "Get a note's full content and metadata",
	Long: `Return full note content including frontmatter, body, headings, and blocks.
Use --frontmatter-only to omit body text.

This is the canonical READ-ONE-NOTE-BY-ID path. It fires access tracking
(reinforcement signal for the activation-based ranking layer), so prefer it
over the Read tool when you want to read a vault note's body. The cleanest
read path is also the tracked one.

EXAMPLES

  vaultmind note get reference-current-context --vault vaultmind-identity
      Print the note's id+type+title header followed by the body inline.

  vaultmind note get reference-current-context --vault vaultmind-identity --json
      Same content, structured envelope. Use from scripts.

  vaultmind note get concept-act-r --vault vaultmind-vault --frontmatter-only
      Header + frontmatter only, no body. Use to inspect tags/related_ids.

PAIRS WELL WITH

  vaultmind ask "X" --pointers-only      # find the right id, then note get it
  vaultmind self                          # see what you've been touching`,
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
