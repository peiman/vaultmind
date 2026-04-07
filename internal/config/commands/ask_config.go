package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// AskMetadata defines metadata for the ask command.
var AskMetadata = config.CommandMetadata{
	Use:          "ask <query>",
	Short:        "Compound search + context-pack: answer 'what do I know about X?'",
	Long:         "Search the vault, pick the top hit, and pack token-budgeted context around it. One command replaces the manual search → recall → summarize chain.",
	ConfigPrefix: "app.ask",
	FlagOverrides: map[string]string{
		"app.ask.vault":        "vault",
		"app.ask.json":         "json",
		"app.ask.budget":       "budget",
		"app.ask.max_items":    "max-items",
		"app.ask.search_limit": "search-limit",
	},
}

// AskOptions returns config options for the ask command.
func AskOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.ask.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.ask.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.ask.budget", DefaultValue: 4000, Description: "Token budget for context-pack", Type: "int"},
		{Key: "app.ask.max_items", DefaultValue: 8, Description: "Max context items", Type: "int"},
		{Key: "app.ask.search_limit", DefaultValue: 5, Description: "Max search hits", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(AskOptions)
}
