package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// askRankingBounds reads the ranking knobs ask already uses. They live here,
// with the rest of app.ask, so note propose can rank the same way without
// reading another command's keys. The propose command has no copies of these
// flags; an unset flag leaves the registry value ask would use.
func askRankingBounds(cmd *cobra.Command) (searchLimit, forwardTokens, salientTerms int) {
	searchLimit = getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit)
	forwardTokens = getConfigValueWithFlags[int](cmd, "query-forward-tokens", config.KeyAppAskQueryForwardTokens)
	salientTerms = getConfigValueWithFlags[int](cmd, "query-salient-terms", config.KeyAppAskQuerySalientTerms)
	return
}

// openAskIndex opens one vault's retriever and embedder. Tests replace it to
// count searches. The per-vault pass and the delivery pass both call it, and
// a given vault path gets the same embedder back: the query embedding is
// memoized on that instance, and a fresh one would forward again.
var openAskIndex = func(cmd *cobra.Command, _ string, db *index.DB) query.AutoRetrieverResult {
	return askRetriever(cmd, db)
}

// askSearchFilters is the type, tag, and path scope ask applies, plus the
// draft switch for this one search. note propose ranks through the same
// filters, so the check and ask agree. The draft switch is an argument:
// the two passes of the retrieval check differ only in that bit.
func askSearchFilters(cmd *cobra.Command, includeDrafts bool) index.SearchFilters {
	return index.SearchFilters{
		Type:          getConfigValueWithFlags[string](cmd, "type", config.KeyAppAskType),
		Tag:           getConfigValueWithFlags[string](cmd, "tag", config.KeyAppAskTag),
		PathPrefix:    getConfigValueWithFlags[string](cmd, "path", config.KeyAppAskPath),
		IncludeDrafts: includeDrafts,
	}
}

// askRetriever builds the vault's retriever, scoped by --type, --tag and --path.
// Every ask path — direct, --read and each federated vault — builds through
// here, so none of them can search unscoped.
func askRetriever(cmd *cobra.Command, db *index.DB) query.AutoRetrieverResult {
	ret := query.BuildAutoRetrieverFull(cmd.Context(), db)
	filters := askSearchFilters(cmd, retrievalIncludeDrafts(cmd))
	if filters != (index.SearchFilters{}) {
		ret.Retriever = query.FilteredRetriever{Base: ret.Retriever, Filters: filters}
	}
	return ret
}

// shownLedgerDir is the state subfolder holding one ledger per conversation.
const shownLedgerDir = "shown"

// shownLedgerRetention is how long a finished conversation's ledger is kept
// before the next dedup-window ask sweeps it.
const shownLedgerRetention = 7 * 24 * time.Hour

// askShown is --dedup-window: the conversation's ledger and how far back it
// counts. The zero value is off.
type askShown struct {
	ledger query.ShownLedger
	window time.Duration
}

// openAskShown reads --dedup-window. Off when empty; a value that is not a
// positive duration is an error rather than a silent off, since the caller
// asked for deduplication and would not see it missing.
func openAskShown(cmd *cobra.Command) (askShown, error) {
	raw := getConfigValueWithFlags[string](cmd, "dedup-window", config.KeyAppAskDedupWindow)
	if raw == "" {
		return askShown{}, nil
	}
	window, err := time.ParseDuration(raw)
	if err != nil || window <= 0 {
		return askShown{}, fmt.Errorf("--dedup-window %q: want a positive duration such as 10m", raw)
	}
	// DataDir, not StateDir: it honours XDG_DATA_HOME on every OS and refuses a
	// test's write to the real directory. StateDir did neither on macOS, and
	// this ledger's first test run landed in the real Application Support.
	dir, err := xdg.DataDir()
	if err != nil {
		return askShown{}, fmt.Errorf("--dedup-window: data directory: %w", err)
	}
	ledgerDir := filepath.Join(dir, shownLedgerDir)
	query.PruneShownLedgers(ledgerDir, shownLedgerRetention, time.Now())
	ledger := query.OpenShownLedger(ledgerDir, os.Getenv(experiment.EnvUserSessionID))
	return askShown{ledger: ledger, window: window}, nil
}

func (s askShown) recent() map[string]bool {
	if s.window == 0 {
		return nil
	}
	return s.ledger.Recent(s.window, time.Now())
}

// record notes what this answer handed over — only when text reached the
// caller, so a pointers-only or withheld answer never marks a note as seen.
func (s askShown) record(result *query.AskResult, pointersOnly, jsonOut bool) {
	if s.window == 0 {
		return
	}
	if delivered, _ := result.DeliveredTo(pointersOnly, jsonOut); !delivered {
		return
	}
	if err := s.ledger.Record(result.DeliveredIDs(), time.Now()); err != nil {
		log.Debug().Err(err).Msg("recording shown notes failed; the next answer may repeat them")
	}
}
