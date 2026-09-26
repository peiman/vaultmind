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

// askRetriever builds the vault's retriever, scoped by --type and --tag. Every
// ask path — direct, --read and each federated vault — builds through here, so
// none of them can search unscoped.
func askRetriever(cmd *cobra.Command, db *index.DB) query.AutoRetrieverResult {
	ret := query.BuildAutoRetrieverFull(cmd.Context(), db)
	filters := index.SearchFilters{
		Type: getConfigValueWithFlags[string](cmd, "type", config.KeyAppAskType),
		Tag:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppAskTag),
	}
	if filters != (index.SearchFilters{}) {
		ret.Retriever = query.FilteredRetriever{Base: ret.Retriever, Filters: filters}
	}
	return ret
}

// shownLedgerDir is the state subfolder holding one ledger per conversation.
const shownLedgerDir = "shown"

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
	ledger := query.OpenShownLedger(filepath.Join(dir, shownLedgerDir), os.Getenv(experiment.EnvUserSessionID))
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
