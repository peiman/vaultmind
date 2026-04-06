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

// LinksNeighborsMetadata defines the metadata for the links neighbors command.
var LinksNeighborsMetadata = config.CommandMetadata{
	Use:          "neighbors <id-or-path>",
	Short:        "Show graph neighborhood",
	Long:         "BFS traversal from a note. Returns nodes with distance and edge info.",
	ConfigPrefix: "app.linksneighbors",
	FlagOverrides: map[string]string{
		"app.linksneighbors.vault":          "vault",
		"app.linksneighbors.json":           "json",
		"app.linksneighbors.depth":          "depth",
		"app.linksneighbors.min_confidence": "min-confidence",
		"app.linksneighbors.max_nodes":      "max-nodes",
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

// LinksNeighborsOptions returns configuration options for the links neighbors command.
func LinksNeighborsOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.linksneighbors.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.linksneighbors.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.linksneighbors.depth", DefaultValue: 1, Description: "Maximum traversal depth", Type: "int"},
		{Key: "app.linksneighbors.min_confidence", DefaultValue: "low", Description: "Minimum edge confidence (low, medium, high)", Type: "string"},
		{Key: "app.linksneighbors.max_nodes", DefaultValue: 200, Description: "Maximum nodes to return", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(LinksOptions)
	config.RegisterOptionsProvider(LinksNeighborsOptions)
}
