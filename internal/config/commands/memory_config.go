package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// MemoryRecallMetadata defines metadata for the memory recall command.
var MemoryRecallMetadata = config.CommandMetadata{
	Use:          "recall <id-or-path>",
	Short:        "Enriched graph neighborhood",
	Long:         "BFS traversal from a note with full frontmatter for each node.",
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
	Use:          "related <id-or-path>",
	Short:        "Related notes by mode",
	Long:         "List notes related to the target, filtered by explicit/inferred/mixed mode.",
	ConfigPrefix: "app.memoryrelated",
	FlagOverrides: map[string]string{
		"app.memoryrelated.vault": "vault",
		"app.memoryrelated.json":  "json",
		"app.memoryrelated.mode":  "mode",
	},
}

// MemoryContextPackMetadata defines metadata for the memory context-pack command.
var MemoryContextPackMetadata = config.CommandMetadata{
	Use:          "context-pack <id-or-path>",
	Short:        "Token-budgeted context assembly",
	Long:         "Assemble a bounded retrieval payload for agent consumption.",
	ConfigPrefix: "app.memorycontextpack",
	FlagOverrides: map[string]string{
		"app.memorycontextpack.vault":  "vault",
		"app.memorycontextpack.json":   "json",
		"app.memorycontextpack.budget": "budget",
		"app.memorycontextpack.depth":  "depth",
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
}
