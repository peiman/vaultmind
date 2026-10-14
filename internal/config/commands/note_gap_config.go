package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteGapMetadata defines metadata for note gap.
var NoteGapMetadata = config.CommandMetadata{
	Use:   "gap <question>",
	Short: "Record a question the vault did not answer",
	Long: `Record an open gap: a note of type gap for a question this vault did not answer.

A gap is a note under gaps/, so it stays in git. The index is a cache and is
not the record. The note stores the question, when it was opened, the session
when VAULTMIND_USER_SESSION_ID is set, and the note ids ask returns for that
question right now. Drafts and gaps are not among those ids.

An open gap with the same question, spacing and case aside, is reported
(mode existing) instead of writing another note. A closed gap does not count.

Gaps are never returned as knowledge. --include-drafts does not reveal them.
note get still reads one. The tree marks it [gap]. doctor lists the open ones.

FLAGS

  --vault <dir>  Path to the vault root.
  --json         Output a v1 envelope.`,
	ConfigPrefix: "app.notegap",
	FlagOverrides: map[string]string{
		"app.notegap.vault": "vault",
		"app.notegap.json":  "json",
	},
}

// NoteGapOptions returns config options for note gap.
func NoteGapOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.notegap.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.notegap.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteGapOptions)
}
