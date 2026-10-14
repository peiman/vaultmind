package cmd

import (
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/gap"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// recordPossibleGap appends a weak or no-match ask to the per-user hint log.
// The log is not a note and it is not in the vault. A write error is not an
// ask error. The raw query is stored only when config or the environment
// opts in. There is no flag for that switch, or for the retained-record limit.
//
// A hook caller is not a hint. Session and recall traffic would otherwise
// fill the log with questions the person did not ask.
func recordPossibleGap(cmd *cobra.Command, vaultPath, queryText string, result *query.AskResult) {
	if result == nil || index.HookCaller() {
		return
	}
	store := getConfigValueWithFlags[bool](cmd, "gap-store-query", config.KeyAppAskGapStoreQuery)
	limit := getConfigValueWithFlags[int](cmd, "gap-log-limit", config.KeyAppAskGapLogLimit)
	err := gap.RecordPossible(vaultPath, queryText, result.TopHitConfidence, store, limit, time.Now())
	if err != nil {
		log.Debug().Err(err).Msg("possible-gap log failed")
	}
}
