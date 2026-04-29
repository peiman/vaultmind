package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SelfMetadata defines the metadata for the self command — the agent's
// own memory-state introspection view (recent / hot / stale).
var SelfMetadata = config.CommandMetadata{
	Use:          "self",
	Short:        "Show your memory state — recent, hot, stale notes",
	Long:         "Render the activation state of your vault: notes touched recently, notes with the highest ACT-R activation, and notes drifting away. First-person AX for the agent using vaultmind as long-term memory.",
	ConfigPrefix: "app.self",
	FlagOverrides: map[string]string{
		"app.self.vault": "vault",
		"app.self.limit": "limit",
	},
}

// SelfOptions returns configuration options for the self command.
func SelfOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.self.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.self.limit", DefaultValue: 10, Description: "Max rows per section (recent/hot/stale)", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(SelfOptions)
}
