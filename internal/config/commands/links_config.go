package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// LinksOutMetadata defines the metadata for the links out command.
var LinksOutMetadata = config.CommandMetadata{
	Use:   "out <id-or-path>",
	Short: "List notes referenced by a note (outbound edges)",
	Long: `Find all notes referenced by the given note (outbound edges).

Shows what a note depends on, cites, or extends — useful for tracing a
note's sources, following its reasoning chain, or finding related material.

Each edge includes its type (e.g., "cites", "extends", "references") and a
confidence label (high, medium, low) reflecting how strongly the reference
was detected.

FLAGS

  --edge-type     Filter to a specific edge type (e.g., "cites"). Omit to show all types.
  --json          Output in machine-readable JSON. Includes source_id and a "links" array
                  with edge_type, confidence, target_id, and resolution status per edge.
  --vault         Path to vault root (default: current directory).

EXAMPLES

  vaultmind links out concept-spreading-activation
      Show all outbound edges from the note, one per line: target  edge_type  confidence.

  vaultmind links out decision-expand-cache --edge-type cites
      Show only edges of type "cites" — the sources this decision depends on.

  vaultmind links out concept-spreading-activation --json
      Machine-readable output; pipe to jq or an agent tool.`,
	ConfigPrefix: "app.links",
	FlagOverrides: map[string]string{
		"app.links.vault":     "vault",
		"app.links.json":      "json",
		"app.links.edge_type": "edge-type",
	},
}

// LinksInMetadata defines the metadata for the links in command.
var LinksInMetadata = config.CommandMetadata{
	Use:   "in <id-or-path>",
	Short: "List notes linking to a note (inbound edges)",
	Long: `Find all notes that reference the given note (inbound edges).

Useful for understanding a note's context and impact: which notes depend on
it, cite it as rationale, or point to it as a concept to explore further.

Each result shows the source note's ID, the edge type, and a confidence label
(high, medium, low) reflecting how strongly the reference was detected.

FLAGS

  --edge-type     Filter to a specific edge type (e.g., "cites"). Omit to show all types.
  --json          Output in machine-readable JSON. Includes target_id and a "links" array
                  with source_id, edge_type, confidence, and resolution status per edge.
  --vault         Path to vault root (default: current directory).

EXAMPLES

  vaultmind links in concept-spreading-activation
      Show all notes that reference spreading-activation, one per line:
      source_id  edge_type  confidence.

  vaultmind links in decision-expand-cache --edge-type cites
      Show only notes that cite this decision as rationale.

  vaultmind links in concept-spreading-activation --json
      Machine-readable output; pipe to jq or an agent tool.`,
	ConfigPrefix: "app.links",
	FlagOverrides: map[string]string{
		"app.links.vault":     "vault",
		"app.links.json":      "json",
		"app.links.edge_type": "edge-type",
	},
}

// LinksNeighborsMetadata defines the metadata for the links neighbors command.
var LinksNeighborsMetadata = config.CommandMetadata{
	Use:   "neighbors <id-or-path>",
	Short: "Explore a note's graph neighborhood via breadth-first traversal",
	Long: `Explore the notes reachable from the given note within N hops (breadth-first traversal).

Useful for finding related material at increasing degrees of separation: direct
references (depth 1), second-order clusters (depth 2), or broader context.
Results are ordered by distance from the starting note and show the connecting
edge type and confidence for each hop.

CONFIDENCE LABELS

  Edges carry a confidence label (high, medium, low) reflecting how strongly
  a reference was detected. Use --min-confidence to prune weak references
  and focus on the most reliable connections.

FLAGS

  --depth           Maximum hops from the starting note (default: 1). Increase
                    to explore further context.
  --min-confidence  Minimum edge confidence to include: high, medium, or low
                    (default: low, includes all edges). Useful for reducing noise.
  --max-nodes       Cap the number of nodes returned (default: 200). If the cap
                    is reached, output notes "(max reached)" so you know the graph
                    was truncated.
  --json            Output in machine-readable JSON. Includes a "nodes" array with
                    id, distance, and edge_from (edge_type, confidence) per node,
                    plus a "max_nodes_reached" boolean.
  --vault           Path to vault root (default: current directory).

EXAMPLES

  vaultmind links neighbors concept-spreading-activation
      Show all notes within 1 hop (direct references), one per line.

  vaultmind links neighbors concept-spreading-activation --depth 2
      Show second-degree connections — notes that cite notes that cite this one.

  vaultmind links neighbors concept-spreading-activation --depth 2 --min-confidence medium
      Second-degree neighborhood, pruned to medium+ confidence edges (reduces noise).

  vaultmind links neighbors concept-spreading-activation --json
      Machine-readable output; pipe to jq or an agent tool.`,
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
