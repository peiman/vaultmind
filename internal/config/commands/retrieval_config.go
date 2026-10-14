package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IncludeDraftsDescription is the help text for the one retrieval switch.
// Ask and search register the flag by hand: the key is not under either
// command prefix, and a second key would split the default.
const IncludeDraftsDescription = "Rank notes whose status is draft. Off by default: a draft stays out of ask and search until its status is changed."

// RetrievalOptions is the shared retrieval policy. One key, read by every
// command that ranks notes.
func RetrievalOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.retrieval.include_drafts",
			DefaultValue: false,
			Description:  IncludeDraftsDescription,
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(RetrievalOptions)
}
