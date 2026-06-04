package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SearchMetadata defines the metadata for the search command.
var SearchMetadata = config.CommandMetadata{
	Use:   "search <query>",
	Short: "Search vault notes by keyword, semantic similarity, or both",
	Long: `Search note titles and body text and return ranked hits.
Keyword mode is the default and needs no index; semantic and hybrid modes
require embeddings built with "vaultmind index --embed".

SEARCH MODES

  keyword (default)  Pure full-text search using SQLite FTS5. Fast, no
                     embeddings needed. Best for exact-term queries.

  semantic           Embedding-based similarity search. Needs: vaultmind index --embed.
                     Best for concept queries where exact wording varies.

  hybrid             Combines keyword and semantic via reciprocal rank fusion (RRF).
                     Best all-around mode when embeddings are available.
                     Needs: vaultmind index --embed.

FLAGS

  --vault string    Path to vault root (default ".")
  --mode string     Search mode: keyword, semantic, or hybrid (default "keyword")
  --type string     Filter results to notes of this type (e.g. concept, source)
  --tag string      Filter results to notes with this tag
  --limit int       Maximum number of results to return (default 20)
  --offset int      Skip the first N results for pagination (default 0)
  --json            Output results as a JSON envelope instead of plain text

OUTPUT INCLUDES

  Default:  one line per hit — id followed by title
  --json:   envelope with query, offset, limit, total, and hits array
            (each hit: id, title, score, type, path)

WHEN TO USE

  search:  You want to browse ranked hits and choose what to read next.
  ask:     You want the top hit plus token-budgeted context already packed —
           one command replaces the search-then-recall chain.

EXAMPLES

  vaultmind search "spreading activation" --vault my-vault
  vaultmind search "memory consolidation" --mode hybrid --vault my-vault       # semantic + keyword
  vaultmind search "attention" --type concept --tag learning --vault my-vault  # filtered
  vaultmind search "retrieval" --limit 5 --json --vault my-vault               # machine-readable top 5
  vaultmind search "recall" --limit 10 --offset 10 --vault my-vault            # page 2`,
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
