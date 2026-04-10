package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SearchMetadata defines the metadata for the search command.
var SearchMetadata = config.CommandMetadata{
	Use:          "search <query>",
	Short:        "Full-text search across vault notes",
	Long:         "Search note titles and body text using SQLite FTS5. Supports --type and --tag filters, --limit and --offset for pagination.",
	ConfigPrefix: "app.search",
	FlagOverrides: map[string]string{
		"app.search.vault":  "vault",
		"app.search.json":   "json",
		"app.search.limit":  "limit",
		"app.search.offset": "offset",
		"app.search.type":   "type",
		"app.search.tag":    "tag",
		"app.search.mode":   "mode",
	},
}

// SearchOptions returns configuration options for the search command.
func SearchOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.search.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.search.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.search.limit", DefaultValue: 20, Description: "Maximum results to return", Type: "int"},
		{Key: "app.search.offset", DefaultValue: 0, Description: "Skip first N results", Type: "int"},
		{Key: "app.search.type", DefaultValue: "", Description: "Filter by note type", Type: "string"},
		{Key: "app.search.tag", DefaultValue: "", Description: "Filter by tag", Type: "string"},
		{Key: "app.search.mode", DefaultValue: "keyword", Description: "Search mode: keyword, semantic, or hybrid", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(SearchOptions)
}
