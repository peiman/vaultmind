package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
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
	paths, err := resolveAskVaultPaths(vaultPath,
		getConfigValueWithFlags[string](cmd, "vaults", config.KeyAppSearchVaults))
	if err != nil {
		return err
	}
	if len(paths) > 1 {
		return runSearchVaults(cmd, args[0], paths)
	}
	_, err = searchVault(cmd, paths[0], args[0], cmd.OutOrStdout(),
		getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson), false)
	return err
}

// searchVault runs one vault's search, rendering to w. With several vaults an
// open failure is returned rather than written: the caller reports it once, for
// the whole search, in the shape the whole search promised.
func searchVault(cmd *cobra.Command, vaultPath, queryText string, w io.Writer, jsonOut, several bool) (*query.SearchResult, error) {
	mode := getConfigValueWithFlags[string](cmd, "mode", config.KeyAppSearchMode)
	var vdb *cmdutil.VaultDB
	var err error
	if several {
		vdb, err = cmdutil.OpenVaultDB(vaultPath)
	} else {
		vdb, err = cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "search")
	}
	if err != nil {
		return nil, err
	}
	defer vdb.Close()

	retriever, cleanup, err := query.BuildRetriever(cmd.Context(), mode, vdb.DB)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	cfg := buildSearchConfig(cmd, queryText, vaultPath)
	cfg.JSONOutput = jsonOut
	result, err := query.RunSearch(retriever, cfg, w)
	logSearchExperiment(cmd, vaultPath, mode, queryText, result, err)
	return result, err
}

// searchVaultResult is one vault's section of a --vaults search.
type searchVaultResult struct {
	Vault  string              `json:"vault"`
	Result *query.SearchResult `json:"result"`
}

// runSearchVaults searches each vault in turn: one ranked section per vault,
// each vault's own ranking — scores from different vaults are not on one scale.
// Nothing is printed until every vault has answered, so a failing vault is one
// error that names it, not half the output followed by a diagnostic.
func runSearchVaults(cmd *cobra.Command, queryText string, paths []string) error {
	if err := requireRealVaults(paths); err != nil {
		return err
	}
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson)
	w := cmd.OutOrStdout()
	sections := make([]searchVaultResult, 0, len(paths))
	var text bytes.Buffer
	for i, p := range paths {
		var rendered bytes.Buffer
		res, err := searchVault(cmd, p, queryText, &rendered, false, true)
		if err != nil {
			if jsonOut {
				return cmdutil.WriteJSONError(w, "search", searchVaultErrorCode, fmt.Sprintf("searching %s: %v", p, err))
			}
			return fmt.Errorf("searching %s: %w", p, err)
		}
		sections = append(sections, searchVaultResult{Vault: p, Result: res})
		writeSearchSection(&text, i, p, res.Total, rendered.Bytes())
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(envelope.OK("search", map[string]any{"vaults": sections}))
	}
	_, err := w.Write(text.Bytes())
	return err
}

// searchVaultErrorCode tags a --vaults search that failed on one of its vaults.
const searchVaultErrorCode = "vault_error"

func writeSearchSection(w io.Writer, i int, vaultPath string, total int, rendered []byte) {
	if i > 0 {
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintf(w, "%s — %d hits\n", vaultPath, total)
	_, _ = w.Write(rendered)
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
// variant payload. Logs regardless of retrieval error so failed retrievals are
// observable (curiosity-over-certainty) and distinguishable from zero-hit
// success via the event_data.error field. Non-blocking — write errors are
// swallowed by design.
func logSearchExperiment(cmd *cobra.Command, vaultPath, mode, queryText string, result *query.SearchResult, retrievalErr error) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)
	hits := toRetrievalHits(result)
	variants := experiment.BuildVariantPayload(mode, hits)
	count := 0
	if result != nil {
		count = result.Total
	}
	_, _ = session.LogSearchEvent(queryText, mode, experiment.BuildRetrievalEventData(variants, count, retrievalErr))
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
			Scores:   h.Components,
		}
	}
	return hits
}
