package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// LinksOutMetadata defines the metadata for the links out command.
var LinksOutMetadata = config.CommandMetadata{
	Use:          "out <id-or-path>",
	Short:        "Show outbound links from a note",
	Long:         "Return all outbound edges from the given note, with edge type and confidence.",
	ConfigPrefix: "app.links",
	FlagOverrides: map[string]string{
		"app.links.vault":     "vault",
		"app.links.json":      "json",
		"app.links.edge_type": "edge-type",
	},
}

// LinksInMetadata defines the metadata for the links in command.
var LinksInMetadata = config.CommandMetadata{
	Use:          "in <id-or-path>",
	Short:        "Show inbound links to a note",
	Long:         "Return all inbound edges pointing to the given note.",
	ConfigPrefix: "app.links",
	FlagOverrides: map[string]string{
		"app.links.vault":     "vault",
		"app.links.json":      "json",
		"app.links.edge_type": "edge-type",
	},
}

// LinksOptions returns configuration options for links commands.
func LinksOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.links.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.links.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.links.edge_type", DefaultValue: "", Description: "Filter by edge type", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(LinksOptions)
}
