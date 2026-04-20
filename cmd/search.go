package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
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

	retriever, cleanup, err := query.BuildRetriever(mode, vdb.DB)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	result, err := query.RunSearch(retriever, buildSearchConfig(cmd, args[0], vaultPath), cmd.OutOrStdout())
	logSearchExperiment(cmd, vaultPath, mode, args[0], result)
	return err
}

// buildSearchConfig assembles the SearchConfig from command flags.
func buildSearchConfig(cmd *cobra.Command, queryText, vaultPath string) query.SearchConfig {
	return query.SearchConfig{
		Query:      queryText,
		Limit:      getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSearchLimit),
		Offset:     getConfigValueWithFlags[int](cmd, "offset", config.KeyAppSearchOffset),
		TypeFilter: getConfigValueWithFlags[string](cmd, "type", config.KeyAppSearchType),
		TagFilter:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppSearchTag),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson),
		VaultPath:  vaultPath,
	}
}

// logSearchExperiment records the search event with retrieval results as the
// variant payload. Non-blocking — errors are swallowed by design.
func logSearchExperiment(cmd *cobra.Command, vaultPath, mode, queryText string, result *query.SearchResult) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)
	_, _ = session.LogSearchEvent(queryText, mode, map[string]any{
		"variants": experiment.BuildVariantPayload(mode, toRetrievalHits(result)),
	})
}

// toRetrievalHits maps search hits to the experiment payload input type.
// Returns nil when result is nil (e.g. retrieval failed before producing hits).
func toRetrievalHits(result *query.SearchResult) []experiment.RetrievalHit {
	if result == nil {
		return nil
	}
	hits := make([]experiment.RetrievalHit, len(result.Hits))
	for i, h := range result.Hits {
		hits[i] = experiment.RetrievalHit{
			NoteID:   h.ID,
			Rank:     i + 1 + result.Offset,
			Score:    h.Score,
			NoteType: h.Type,
			Path:     h.Path,
		}
	}
	return hits
}
