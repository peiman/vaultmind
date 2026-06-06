package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// MemoryRecallMetadata defines metadata for the memory recall command.
var MemoryRecallMetadata = config.CommandMetadata{
	Use:   "recall <id-or-path>",
	Short: "Traverse the graph from a note and return neighbors with full frontmatter",
	Long: `Perform a breadth-first traversal starting from a target note and return all
neighbors within the specified depth. Every node in the result carries its
full frontmatter, so callers get a rich, typed neighborhood without follow-up
note-get calls.

BFS (breadth-first search) means depth=1 returns only direct neighbors,
depth=2 includes their neighbors as well, and so on.

CONFIDENCE LEVELS

  Edges in the vault carry a confidence level (low, medium, high).
  --min-confidence sets the floor: only edges at or above that level are
  traversed. The default is "high" so recall stays precise; lower it to
  "medium" or "low" to include weaker, LLM-inferred relationships.

FLAGS

  --depth <n>
      Maximum traversal depth. Default: 1.

  --max-nodes <n>
      Safety limit on result size. Traversal stops when N nodes have been
      collected. The JSON output includes a "max_nodes_reached" field when
      the limit was hit. Default: 50.

  --min-confidence <low|medium|high>
      Minimum edge confidence required to follow an edge. Default: high.

  --vault <path>
      Path to vault root. Default: ".".

  --json
      Output as a JSON envelope instead of text.

EXAMPLES

  vaultmind memory recall concept-spreading-activation
      # direct neighbors (depth 1) at high confidence

  vaultmind memory recall concept-spreading-activation --depth 2 --max-nodes 20
      # up to depth 2, capped at 20 nodes

  vaultmind memory recall concept-spreading-activation --min-confidence medium --json
      # include medium-confidence edges, machine-readable output

WHEN TO USE

  Use recall when you want the full neighborhood around a known note — for
  populating a reasoning window or checking what is explicitly linked.
  For a flat list of edge-typed relations, use "memory related" instead.
  For a token-budgeted payload ready for agent consumption, use
  "memory context-pack" instead.`,
	ConfigPrefix: "app.memoryrecall",
	FlagOverrides: map[string]string{
		"app.memoryrecall.vault":          "vault",
		"app.memoryrecall.json":           "json",
		"app.memoryrecall.depth":          "depth",
		"app.memoryrecall.min_confidence": "min-confidence",
		"app.memoryrecall.max_nodes":      "max-nodes",
	},
}

// MemoryRelatedMetadata defines metadata for the memory related command.
var MemoryRelatedMetadata = config.CommandMetadata{
	Use:   "related <id-or-path>",
	Short: "List notes related to the target, filtered by edge type",
	Long: `Return all notes directly related to the target, with each result showing
the edge type and confidence level. Use --mode to filter by how the
relationship was established.

MODES

  explicit
      Only edges recorded in the note's frontmatter (related_ids, parent_id,
      source_ids), wikilinks [[...]], and markdown links [...]. These are
      user-curated and carry high confidence.

  inferred
      Only LLM-inferred edges. These represent relationships the model
      detected during indexing but that the user has not explicitly linked.
      They carry medium or low confidence and are useful for discovery.

  mixed
      Both explicit and inferred edges, ranked by confidence and edge type.
      This is the default.

FLAGS

  --mode <explicit|inferred|mixed>
      Which edge types to include. Default: mixed.

  --vault <path>
      Path to vault root. Default: ".".

  --json
      Output as a JSON envelope instead of text.

EXAMPLES

  vaultmind memory related concept-spreading-activation
      # all related notes (explicit + inferred)

  vaultmind memory related concept-spreading-activation --mode explicit
      # only notes with explicit frontmatter or wikilink edges

  vaultmind memory related concept-spreading-activation --mode inferred --json
      # LLM-inferred suggestions only, machine-readable

WHEN TO USE

  Use related for a flat list of directly linked notes when you already know
  the target. For a depth-traversal that follows edges transitively, use
  "memory recall". For a token-budgeted agent payload, use
  "memory context-pack".`,
	ConfigPrefix: "app.memoryrelated",
	FlagOverrides: map[string]string{
		"app.memoryrelated.vault": "vault",
		"app.memoryrelated.json":  "json",
		"app.memoryrelated.mode":  "mode",
	},
}

// MemoryContextPackMetadata defines metadata for the memory context-pack command.
var MemoryContextPackMetadata = config.CommandMetadata{
	Use:   "context-pack <id-or-path>",
	Short: "Pack the target note and ranked context items within a token budget",
	Long: `Load a target note, then fill a token budget with its ranked neighbors,
stopping when the budget is exhausted. Designed to produce an agent-ready
payload: the target is always included, followed by context items in
priority order (explicit relations first, then inferred edges).

TOKEN BUDGET

  The budget is a soft limit on tokens. Context items are added one by one
  in ranked order; as soon as adding the next item would exceed the budget,
  it is skipped and the "truncated" field in the output is set to true. The
  target note itself is always included regardless of its size.

  Use --slim to reduce the token footprint of context item frontmatter when
  the budget is tight.

FLAGS

  --budget <tokens>
      Token budget for context assembly. Default: 4096.

  --depth <n>
      BFS traversal depth for finding candidate context items. Default: 1
      (direct neighbors only).

  --max-items <n>
      Maximum number of context items to include. 0 means no limit beyond
      the token budget. Default: 0.

  --slim
      Reduce each context item's frontmatter to {type, title, status} only,
      saving tokens for more items.

  --vault <path>
      Path to vault root. Default: ".".

  --json
      Output as a JSON envelope instead of text.

OUTPUT INCLUDES

  target:        the target note's frontmatter and body
  context:       ranked context items, each with frontmatter (full or slim)
  used_tokens:   tokens consumed
  budget_tokens: the requested budget
  truncated:     true if one or more items were dropped due to budget

EXAMPLES

  vaultmind memory context-pack concept-spreading-activation
      # default 4096-token pack around the target

  vaultmind memory context-pack concept-spreading-activation --budget 2000 --slim
      # tight budget, slim frontmatter to fit more items

  vaultmind memory context-pack concept-spreading-activation --max-items 8 --slim --json
      # cap at 8 items, slim, machine-readable (preferred for agent hooks)

WHEN TO USE

  Use context-pack when an agent needs a self-contained, token-bounded
  working context around a topic. For a full neighborhood traversal without
  a budget, use "memory recall". For a flat list of related notes, use
  "memory related".`,
	ConfigPrefix: "app.memorycontextpack",
	FlagOverrides: map[string]string{
		"app.memorycontextpack.vault":     "vault",
		"app.memorycontextpack.json":      "json",
		"app.memorycontextpack.budget":    "budget",
		"app.memorycontextpack.depth":     "depth",
		"app.memorycontextpack.max_items": "max-items",
		"app.memorycontextpack.slim":      "slim",
	},
}

// MemoryRecallOptions returns config options for memory recall.
func MemoryRecallOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memoryrecall.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memoryrecall.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memoryrecall.depth", DefaultValue: 1, Description: "Maximum traversal depth", Type: "int"},
		{Key: "app.memoryrecall.min_confidence", DefaultValue: "high", Description: "Minimum edge confidence (low, medium, high)", Type: "string"},
		{Key: "app.memoryrecall.max_nodes", DefaultValue: 50, Description: "Maximum nodes to return", Type: "int"},
	}
}

// MemoryRelatedOptions returns config options for memory related.
func MemoryRelatedOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memoryrelated.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memoryrelated.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memoryrelated.mode", DefaultValue: "mixed", Description: "Filter mode (explicit, inferred, mixed)", Type: "string"},
	}
}

// MemoryContextPackOptions returns config options for memory context-pack.
func MemoryContextPackOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memorycontextpack.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memorycontextpack.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memorycontextpack.budget", DefaultValue: 4096, Description: "Token budget", Type: "int"},
		{Key: "app.memorycontextpack.depth", DefaultValue: 1, Description: "BFS traversal depth (1 = direct neighbors only)", Type: "int"},
		{Key: "app.memorycontextpack.max_items", DefaultValue: 0, Description: "Max context items (0 = unlimited)", Type: "int"},
		{Key: "app.memorycontextpack.slim", DefaultValue: false, Description: "Slim frontmatter (type, title, status only)", Type: "bool"},
	}
}

// MemoryLinksMetadata defines metadata for the memory links command.
// It unifies directed wikilink-edge queries: outbound (--out), inbound
// (--in, backlinks), or both (--both, default). Absorbs the old top-level
// `links out` and `links in` commands into one direction-flagged command.
var MemoryLinksMetadata = config.CommandMetadata{
	Use:   "links <id-or-path>",
	Short: "List directed wikilink edges for a note (outbound, inbound, or both)",
	Long: `List the directed link edges of a note. By default shows both directions;
use --out for outbound edges only or --in for inbound edges (backlinks) only.

Outbound edges are the notes this note references, cites, or extends.
Inbound edges (backlinks) are the notes that reference this one — useful for
finding callers, dependents, or consumers of a concept.

Each edge includes its type (e.g., "cites", "extends", "references") and a
confidence label (high, medium, low) reflecting how strongly it was detected.

FLAGS

  --out           Show only outbound edges (notes this note references).
  --in            Show only inbound edges (backlinks: notes that reference this one).
  --both          Show both directions (default). Mutually exclusive with --out/--in.
  --edge-type     Filter to a specific edge type (e.g., "cites"). Omit to show all types.
  --json          Output in machine-readable JSON.
  --vault         Path to vault root (default: current directory).

EXAMPLES

  vaultmind memory links concept-spreading-activation
      Show both inbound and outbound edges for the note.

  vaultmind memory links concept-spreading-activation --out
      Show only outbound edges (what the note references).

  vaultmind memory links concept-spreading-activation --in
      Show only backlinks (what references the note).

  vaultmind memory links concept-spreading-activation --out --json
      Machine-readable output; pipe to jq or an agent tool.`,
	ConfigPrefix: "app.memorylinks",
	FlagOverrides: map[string]string{
		"app.memorylinks.vault":     "vault",
		"app.memorylinks.json":      "json",
		"app.memorylinks.edge_type": "edge-type",
		"app.memorylinks.out":       "out",
		"app.memorylinks.in":        "in",
		"app.memorylinks.both":      "both",
	},
}

// MemoryNeighborsMetadata defines metadata for the memory neighbors command.
// It merges the old `links neighbors` (BFS traversal) and `memory recall`
// (full-frontmatter enrichment) into one: BFS to a depth limit, returning
// every reachable node with its full frontmatter.
var MemoryNeighborsMetadata = config.CommandMetadata{
	Use:   "neighbors <id-or-path>",
	Short: "Traverse the graph from a note (BFS) and return neighbors with full frontmatter",
	Long: `Perform a breadth-first traversal starting from a target note and return all
neighbors within the specified depth. Every node carries its full frontmatter,
so callers get a rich, typed neighborhood without follow-up note-get calls.

BFS (breadth-first search) means depth=1 returns only direct neighbors,
depth=2 includes their neighbors as well, and so on.

CONFIDENCE LEVELS

  Edges carry a confidence level (low, medium, high). --min-confidence sets
  the floor: only edges at or above that level are traversed. The default is
  "high" so traversal stays precise; lower it to include weaker, LLM-inferred
  relationships.

FLAGS

  --depth <n>            Maximum traversal depth. Default: 1.
  --max-nodes <n>        Safety limit on result size. Default: 50.
  --min-confidence <l>   Minimum edge confidence (low, medium, high). Default: high.
  --vault <path>         Path to vault root. Default: ".".
  --json                 Output as a JSON envelope instead of text.

EXAMPLES

  vaultmind memory neighbors concept-spreading-activation
      # direct neighbors (depth 1) at high confidence, with full frontmatter

  vaultmind memory neighbors concept-spreading-activation --depth 2 --max-nodes 20
      # up to depth 2, capped at 20 nodes

  vaultmind memory neighbors concept-spreading-activation --min-confidence medium --json
      # include medium-confidence edges, machine-readable output

WHEN TO USE

  Use neighbors when you want the full enriched neighborhood around a known
  note. For a flat list of edge-typed relations, use "memory related". For a
  token-budgeted payload ready for agent consumption, use "memory pack".`,
	ConfigPrefix: "app.memoryneighbors",
	FlagOverrides: map[string]string{
		"app.memoryneighbors.vault":          "vault",
		"app.memoryneighbors.json":           "json",
		"app.memoryneighbors.depth":          "depth",
		"app.memoryneighbors.min_confidence": "min-confidence",
		"app.memoryneighbors.max_nodes":      "max-nodes",
	},
}

// MemoryPackMetadata defines metadata for the memory pack command.
// It is a rename of the old `memory context-pack` (identical behavior).
var MemoryPackMetadata = config.CommandMetadata{
	Use:   "pack <id-or-path>",
	Short: "Pack the target note and ranked context items within a token budget",
	Long: `Load a target note, then fill a token budget with its ranked neighbors,
stopping when the budget is exhausted. Designed to produce an agent-ready
payload: the target is always included, followed by context items in
priority order (explicit relations first, then inferred edges).

TOKEN BUDGET

  The budget is a soft limit on tokens. Context items are added one by one
  in ranked order; as soon as adding the next item would exceed the budget,
  it is skipped and the "truncated" field in the output is set to true. The
  target note itself is always included regardless of its size.

  Use --slim to reduce the token footprint of context item frontmatter when
  the budget is tight.

FLAGS

  --budget <tokens>   Token budget for context assembly. Default: 4096.
  --depth <n>         BFS traversal depth for candidate context items. Default: 1.
  --max-items <n>     Max context items. 0 means no limit beyond the budget. Default: 0.
  --slim              Reduce each context item's frontmatter to {type, title, status}.
  --vault <path>      Path to vault root. Default: ".".
  --json              Output as a JSON envelope instead of text.

EXAMPLES

  vaultmind memory pack concept-spreading-activation
      # default 4096-token pack around the target

  vaultmind memory pack concept-spreading-activation --budget 2000 --slim
      # tight budget, slim frontmatter to fit more items

  vaultmind memory pack concept-spreading-activation --max-items 8 --slim --json
      # cap at 8 items, slim, machine-readable (preferred for agent hooks)

WHEN TO USE

  Use pack when an agent needs a self-contained, token-bounded working context
  around a topic. For a full neighborhood traversal without a budget, use
  "memory neighbors". For a flat list of related notes, use "memory related".`,
	ConfigPrefix: "app.memorypack",
	FlagOverrides: map[string]string{
		"app.memorypack.vault":     "vault",
		"app.memorypack.json":      "json",
		"app.memorypack.budget":    "budget",
		"app.memorypack.depth":     "depth",
		"app.memorypack.max_items": "max-items",
		"app.memorypack.slim":      "slim",
	},
}

// MemoryLinksOptions returns config options for memory links.
func MemoryLinksOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memorylinks.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memorylinks.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memorylinks.edge_type", DefaultValue: "", Description: "Filter by edge type", Type: "string"},
		{Key: "app.memorylinks.out", DefaultValue: false, Description: "Show only outbound edges", Type: "bool"},
		{Key: "app.memorylinks.in", DefaultValue: false, Description: "Show only inbound edges (backlinks)", Type: "bool"},
		{Key: "app.memorylinks.both", DefaultValue: false, Description: "Show both inbound and outbound edges (default)", Type: "bool"},
	}
}

// MemoryNeighborsOptions returns config options for memory neighbors.
func MemoryNeighborsOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memoryneighbors.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memoryneighbors.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memoryneighbors.depth", DefaultValue: 1, Description: "Maximum traversal depth", Type: "int"},
		{Key: "app.memoryneighbors.min_confidence", DefaultValue: "high", Description: "Minimum edge confidence (low, medium, high)", Type: "string"},
		{Key: "app.memoryneighbors.max_nodes", DefaultValue: 50, Description: "Maximum nodes to return", Type: "int"},
	}
}

// MemoryPackOptions returns config options for memory pack.
func MemoryPackOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memorypack.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memorypack.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memorypack.budget", DefaultValue: 4096, Description: "Token budget", Type: "int"},
		{Key: "app.memorypack.depth", DefaultValue: 1, Description: "BFS traversal depth (1 = direct neighbors only)", Type: "int"},
		{Key: "app.memorypack.max_items", DefaultValue: 0, Description: "Max context items (0 = unlimited)", Type: "int"},
		{Key: "app.memorypack.slim", DefaultValue: false, Description: "Slim frontmatter (type, title, status only)", Type: "bool"},
	}
}

// MemorySummarizeMetadata defines metadata for the memory summarize command.
var MemorySummarizeMetadata = config.CommandMetadata{
	Use:          "summarize [id1 id2 ...]",
	Short:        "Assemble note material for agent synthesis",
	Long:         "Load frontmatter and body excerpts from a set of notes. Agents use this output to create reflection notes. Not an LLM call — data assembly only.",
	ConfigPrefix: "app.memorysummarize",
	FlagOverrides: map[string]string{
		"app.memorysummarize.vault":        "vault",
		"app.memorysummarize.json":         "json",
		"app.memorysummarize.ids":          "ids",
		"app.memorysummarize.include_body": "include-body",
		"app.memorysummarize.max_body_len": "max-body-len",
	},
}

// MemorySummarizeOptions returns config options for memory summarize.
func MemorySummarizeOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.memorysummarize.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.memorysummarize.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.memorysummarize.ids", DefaultValue: "", Description: "Comma-separated note IDs (alternative to positional args)", Type: "string"},
		{Key: "app.memorysummarize.include_body", DefaultValue: false, Description: "Include body text excerpts", Type: "bool"},
		{Key: "app.memorysummarize.max_body_len", DefaultValue: 0, Description: "Max body chars per note (0 = full)", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(MemoryRecallOptions)
	config.RegisterOptionsProvider(MemoryRelatedOptions)
	config.RegisterOptionsProvider(MemoryContextPackOptions)
	config.RegisterOptionsProvider(MemorySummarizeOptions)
	config.RegisterOptionsProvider(MemoryLinksOptions)
	config.RegisterOptionsProvider(MemoryNeighborsOptions)
	config.RegisterOptionsProvider(MemoryPackOptions)
}
