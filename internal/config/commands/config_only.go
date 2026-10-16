package commands

// configOnlyKeys are options read from the config file or the environment,
// never from a command-line flag. A per-invocation flag is the wrong place
// for a privacy opt-in: the vault decides, not one call. The list lives in
// project code, not in the framework's ConfigOption, so a framework update
// cannot drop it.
var configOnlyKeys = map[string]bool{
	"app.ask.gap_store_query": true,
	"app.ask.gap_log_limit":   true,
}

// ConfigOnly reports whether key is set only in the config file or the
// environment, with no command-line flag.
func ConfigOnly(key string) bool { return configOnlyKeys[key] }
