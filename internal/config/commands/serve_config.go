// internal/config/commands/serve_config.go
//
// Serve command configuration: metadata + options. The single source of
// truth for `vaultmind serve`.

package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ServeMetadata defines the serve command.
var ServeMetadata = config.CommandMetadata{
	Use:   "serve",
	Short: "Keep vaultmind warm so ask answers faster (started for you)",
	Long: "Keep one vaultmind process running with the embedding model loaded, and answer " +
		"`vaultmind ask` from it: about 0.6 s faster per ask, the time a new process spends " +
		"loading the model.\n\n" +
		"You don't need to run this. The first ask starts it in the background, and later asks " +
		"use it. Each ask still runs the full command, in its own directory and environment and " +
		"against the index as it is now, so it answers exactly what it would without the server. " +
		"If the server is gone or slow, ask runs on its own.\n\n" +
		"One server runs per user per installed binary, shared by every vault and project; it " +
		"listens on a private socket in the state directory and exits after --idle without a " +
		"request, releasing the model's memory. VAULTMIND_NO_SERVE=1 turns it off.",
	ConfigPrefix: "app.serve",
	FlagOverrides: map[string]string{
		"app.serve.idle": "idle",
	},
	SeeAlso: []string{"ask"},
}

// ServeOptions returns the serve command's options.
func ServeOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.serve.idle", DefaultValue: "30m", Description: "Exit after this long without a request (a Go duration: 30m, 2h)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ServeOptions)
}
