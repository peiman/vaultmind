package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var searchCmd = MustNewCommand(commands.SearchMetadata, runSearch)

func init() {
	MustAddToRoot(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind search <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSearchVault)
	mode := getConfigValueWithFlags[string](cmd, "mode", config.KeyAppSearchMode)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "search")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	retriever, cleanup, err := buildRetriever(mode, vdb.DB)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return query.RunSearch(retriever, query.SearchConfig{
		Query:      args[0],
		Limit:      getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSearchLimit),
		Offset:     getConfigValueWithFlags[int](cmd, "offset", config.KeyAppSearchOffset),
		TypeFilter: getConfigValueWithFlags[string](cmd, "type", config.KeyAppSearchType),
		TagFilter:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppSearchTag),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson),
		VaultPath:  vaultPath,
	}, cmd.OutOrStdout())
}

// buildRetriever creates the appropriate retriever for the given search mode.
// Returns a cleanup function that must be deferred if non-nil.
func buildRetriever(mode string, db *index.DB) (query.Retriever, func(), error) {
	switch mode {
	case "keyword", "":
		return &query.FTSRetriever{DB: db}, nil, nil
	case "semantic":
		has, err := index.HasEmbeddings(db)
		if err != nil {
			return nil, nil, fmt.Errorf("checking embeddings: %w", err)
		}
		if !has {
			return nil, nil, fmt.Errorf("no embeddings found — run 'vaultmind index --embed' first")
		}
		embedder, err := newSearchEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &query.EmbeddingRetriever{DB: db, Embedder: embedder}, func() { _ = embedder.Close() }, nil
	case "hybrid":
		has, err := index.HasEmbeddings(db)
		if err != nil {
			return nil, nil, fmt.Errorf("checking embeddings: %w", err)
		}
		if !has {
			return nil, nil, fmt.Errorf("no embeddings found — run 'vaultmind index --embed' first")
		}
		embedder, err := newSearchEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &query.HybridRetriever{
			Retrievers: []query.Retriever{
				&query.FTSRetriever{DB: db},
				&query.EmbeddingRetriever{DB: db, Embedder: embedder},
			},
			K: 60,
		}, func() { _ = embedder.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unknown search mode %q (use keyword, semantic, or hybrid)", mode)
	}
}

func newSearchEmbedder() (*embedding.HugotEmbedder, error) {
	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     filepath.Join(os.Getenv("HOME"), ".vaultmind", "models"),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	return embedder, nil
}
