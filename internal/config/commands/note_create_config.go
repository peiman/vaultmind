package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteCreateMetadata defines metadata for the note create command.
var NoteCreateMetadata = config.CommandMetadata{
	Use:          "create <path> --type <type>",
	Short:        "Create a note from template",
	Long:         "Create a new note from the type's template with variable substitution and field overrides.",
	ConfigPrefix: "app.notecreate",
	FlagOverrides: map[string]string{
		"app.notecreate.vault":  "vault",
		"app.notecreate.json":   "json",
		"app.notecreate.type":   "type",
		"app.notecreate.body":   "body",
		"app.notecreate.commit": "commit",
	},
}

// NoteCreateOptions returns config options for note create.
func NoteCreateOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.notecreate.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.notecreate.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.notecreate.type", DefaultValue: "", Description: "Note type (required)", Type: "string"},
		{Key: "app.notecreate.body", DefaultValue: "", Description: "Body text (overrides template body)", Type: "string"},
		{Key: "app.notecreate.commit", DefaultValue: false, Description: "Stage and commit", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteCreateOptions)
}
