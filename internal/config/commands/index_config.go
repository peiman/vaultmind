package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IndexMetadata defines the metadata for the index command.
var IndexMetadata = config.CommandMetadata{
	Use:   "index",
	Short: "Scan and index vault notes into SQLite, with optional embedding",
	Long: `Scan the vault directory and update the SQLite index so search and recall
commands see current note contents. By default only re-parses notes whose
content has changed since the last run (incremental). Use --full to purge and
rebuild the entire index from scratch.

FLAGS

  --vault path        Vault root directory to index (default: current dir).
  --full              Force a complete rebuild; purges the index first and
                      re-parses every note. Shows deleted count explicitly.
  --embed             Compute and store embeddings for all note bodies after
                      indexing. Required for hybrid (semantic+keyword) search.
  --model name        Embedding model to use with --embed.
                      "minilm"  — 384-dimensional, fast, pure-Go build.
                      "bge-m3"  — 1024-dimensional, dense+sparse+ColBERT
                                  (3-in-1), ORT-tagged build only.
                      Empty (default): auto-selects bge-m3 on ORT builds,
                      minilm on pure-Go builds.
  --allow-slow-backend  Allow BGE-M3 on the pure-Go hugot backend. Omit this
                      flag and the command refuses with a warning — BGE-M3 on
                      pure-Go takes hours for a medium vault.
  --json              Machine-readable JSON output (envelope format).

EMBEDDING MODELS

  minilm:   Default on pure-Go builds. Fast. Produces 384-dimensional dense
            vectors only. Sufficient for basic semantic search.

  bge-m3:   Default on ORT-tagged builds ("task build:ort"). Produces
            dense (1024d) + sparse + ColBERT vectors in one pass, enabling
            the full 4-way RRF hybrid used by "ask" and "search". Recommended
            for recall quality. Requires an ORT binary; pure-Go falls back to
            minilm silently unless --allow-slow-backend is passed.

OUTPUT INCLUDES

  Human-readable (default):
    "Indexed N notes (M skipped, A added, U updated, D deleted)"
    "Embedded N notes (M skipped, E errors) [model: <name>]"
    Warning line if any notes produced empty sparse/ColBERT output.

  JSON (--json):
    Envelope with index stats (indexed, added, updated, deleted, errors,
    full_rebuild) and, when --embed, embed stats (embedded, skipped, errors,
    model).

EXAMPLES

  vaultmind index                                           # incremental update
  vaultmind index --full                                    # full rebuild from scratch
  vaultmind index --embed                                   # incremental + compute embeddings
  vaultmind index --embed --model minilm                    # force minilm model explicitly
  vaultmind index --embed --model bge-m3                    # bge-m3 (ORT build required)
  vaultmind index --vault ./my-vault --embed --json         # JSON output for scripting`,
	ConfigPrefix: "app.index",
	FlagOverrides: map[string]string{
		"app.index.vault":              "vault",
		"app.index.json":               "json",
		"app.index.full":               "full",
		"app.index.embed":              "embed",
		"app.index.model":              "model",
		"app.index.allow_slow_backend": "allow-slow-backend",
	},
}

// IndexOptions returns configuration options for the index command.
func IndexOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.index.vault",
			DefaultValue: ".",
			Description:  "Path to the vault root directory",
			Type:         "string",
			Required:     false,
			Example:      "./my-vault",
		},
		{
			Key:          "app.index.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
		{
			Key:          "app.index.full",
			DefaultValue: false,
			Description:  "Force full rebuild instead of incremental index",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
		{
			Key:          "app.index.embed",
			DefaultValue: false,
			Description:  "Compute and store embeddings for note bodies",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
		{
			Key:          "app.index.model",
			DefaultValue: "",
			Description:  "Embedding model: minilm (384d, fast) or bge-m3 (1024d, 3-in-1). Empty (default) auto-selects: bge-m3 on ORT-tagged builds, minilm on pure-Go.",
			Type:         "string",
			Required:     false,
			Example:      "bge-m3",
		},
		{
			Key:          "app.index.allow_slow_backend",
			DefaultValue: false,
			Description:  "Allow BGE-M3 indexing on the pure-Go backend (hours for medium vaults)",
			Type:         "bool",
			Required:     false,
			Example:      "true",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(IndexOptions)
}
