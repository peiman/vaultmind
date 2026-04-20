package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var askCmd = MustNewCommand(commands.AskMetadata, runAsk)

func init() {
	MustAddToRoot(askCmd)
}

// retrievalModeLabel reports the retriever kind for event logging. Ask uses an
// auto-selected retriever — hybrid when embeddings are available, keyword
// otherwise. Embedder presence is the signal.
func retrievalModeLabel(r query.AutoRetrieverResult) string {
	if r.Embedder != nil {
		return "hybrid"
	}
	return "keyword"
}

// writeZeroHitDiagnostics emits user-facing hints when ask returns no hits.
// Non-fatal: a database error fetching titles is logged at debug and the
// function proceeds. The keyword-only hint always fires first when
// applicable; the title-suggestions block follows when matches exist.
func writeZeroHitDiagnostics(w io.Writer, db *index.DB, queryText, mode string, hitCount int) {
	query.WriteKeywordOnlyHint(w, mode, hitCount)
	if hitCount > 0 {
		return
	}
	titles, err := db.AllNoteTitles()
	if err != nil {
		log.Debug().Err(err).Msg("could not load titles for zero-hit fallback")
		return
	}
	query.WriteTitleSuggestions(w, query.FuzzyTitleMatches(queryText, titles, 3))
}

func runAsk(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind ask <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppAskVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "ask")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	ret := query.BuildAutoRetrieverFull(vdb.DB)
	defer ret.Cleanup()

	resolver := graph.NewResolver(vdb.DB)
	delta := getConfigValueWithFlags[float64](cmd, "activation-delta", config.KeyExperimentsActivationDelta)
	activationScores := computeActivationScores(cmd.Context(), nil, delta)

	result, err := query.Ask(cmd.Context(), ret.Retriever, resolver, vdb.DB, query.AskConfig{
		Query:            args[0],
		Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit:      getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
		ActivationScores: activationScores,
		Embedder:         ret.Embedder,
		ActivationFunc: func(sims map[string]float64) map[string]float64 {
			return computeActivationScores(cmd.Context(), sims, delta)
		},
	})

	mode := retrievalModeLabel(ret)
	logAskExperiment(cmd, args[0], vaultPath, mode, result, err)

	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		if err := query.FormatAsk(result, cmd.OutOrStdout()); err != nil {
			return err
		}
		writeZeroHitDiagnostics(cmd.OutOrStdout(), vdb.DB, args[0], mode, len(result.TopHits))
		return nil
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
