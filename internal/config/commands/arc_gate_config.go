package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ArcGateMetadata defines the opt-in action-time arc gate used by agent hooks.
var ArcGateMetadata = config.CommandMetadata{
	Use:   "gate",
	Short: "Deliver one matching arc at the moment of action",
	Long: "Read a hook payload from stdin and, when an opt-in arc trigger matches, " +
		"deny or block once per session with the arc's Principle. Failures allow the " +
		"action and are reported on stderr so this command is safe to use from hooks.",
	ConfigPrefix: "app.arc.gate",
	FlagOverrides: map[string]string{
		"app.arc.gate.vault": "vault",
		"app.arc.gate.event": "event",
	},
}

// ArcGateOptions returns configuration options for `arc gate`.
func ArcGateOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.arc.gate.vault", DefaultValue: ".", Description: "Path to the vault root", Type: "string"},
		{Key: "app.arc.gate.event", DefaultValue: "", Description: "Hook event: pretooluse or stop", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ArcGateOptions)
}
