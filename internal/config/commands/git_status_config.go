package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// GitStatusMetadata defines the metadata for the git status command.
var GitStatusMetadata = config.CommandMetadata{
	Use:          "status",
	Short:        "Git repository state for VaultMind policies",
	Long:         "Report git repo state relevant to VaultMind mutation policies: branch, dirty files, merge/rebase status.",
	ConfigPrefix: "app.gitstatus",
	FlagOverrides: map[string]string{
		"app.gitstatus.vault": "vault",
		"app.gitstatus.json":  "json",
	},
}

// GitStatusOptions returns configuration options for git status.
func GitStatusOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.gitstatus.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.gitstatus.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(GitStatusOptions)
}
