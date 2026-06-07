package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DocsCommandsMetadata defines the metadata for `docs commands` — emits the
// grouped command catalog (every user-facing command, with its when-to-use) as
// markdown. This is the COMMANDS.md generator: the same catalog that backs
// `vaultmind help` and the embedded onboarding doc, rendered to markdown.
var DocsCommandsMetadata = config.CommandMetadata{
	Use:   "commands",
	Short: "Generate the grouped command reference (COMMANDS.md)",
	Long: `Generate a markdown reference of every user-facing command, grouped by intent,
each with its when-to-use trigger.

This is the same catalog that backs 'vaultmind help' and the embedded
onboarding doc — one generator, rendered to markdown here. Use --output to write
COMMANDS.md; with no --output it prints to stdout.`,
	ConfigPrefix: "app.docs_commands",
	FlagOverrides: map[string]string{
		"app.docs_commands.output": "output",
	},
	Examples: []string{
		"docs commands",
		"docs commands --output internal/onboard/COMMANDS.md",
	},
}

// DocsCommandsOptions returns configuration options for `docs commands`.
func DocsCommandsOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.docs_commands.output",
			DefaultValue: "",
			Description:  "Output file for the command reference (defaults to stdout)",
			Type:         "string",
			Example:      "internal/onboard/COMMANDS.md",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(DocsCommandsOptions)
}
