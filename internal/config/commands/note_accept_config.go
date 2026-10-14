package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteAcceptMetadata defines metadata for note accept.
var NoteAcceptMetadata = config.CommandMetadata{
	Use:   "accept <id>",
	Short: "Promote a draft into recall",
	Long: `Promote a draft written by note propose. Accepting is how a person
says the draft belongs in recall.

  - A new draft flips status to the schema's published value (accepted when
    the type lists it or lists nothing, active when that is the published
    word). Same file, same id.
  - A patch draft appends its body under a heading of its question on the
    target note, keeps the target's frontmatter, adds sources when the
    target's type allows that field, and deletes the proposal. One copy remains.
  - A note that is not a draft is refused. A patch whose target is gone is refused.

The notes that changed are re-indexed, and embedded when embed-on-write is on.

FLAGS

  --vault <dir>   Path to the vault root.
  --json          Output a v1 envelope.`,
	ConfigPrefix: "app.noteaccept",
	FlagOverrides: map[string]string{
		"app.noteaccept.vault": "vault",
		"app.noteaccept.json":  "json",
	},
}

// NoteAcceptOptions returns config options for note accept.
func NoteAcceptOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.noteaccept.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.noteaccept.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteAcceptOptions)
}
