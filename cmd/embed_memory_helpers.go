package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memguard"
	"github.com/spf13/cobra"
)

// embedMaxRSSFlag names the setting for getConfigValueWithFlags. No command
// declares it as a flag, so the value comes from the config file or
// VAULTMIND_APP_EMBED_MAX_RSS_MB.
const embedMaxRSSFlag = "embed-max-rss-mb"

// embedRSS reads this process's memory for the embed guard. A variable for tests.
var embedRSS = memguard.PeakRSS

// embedGuardEvery is how often the guard reads memory. A variable for tests.
var embedGuardEvery = time.Second

// exitOverMemory ends the process when an embed pass passes its memory limit.
// It exits rather than cancels: a runaway forward pass cannot be interrupted,
// and each committed batch is already saved. A variable for tests.
var exitOverMemory = func(w io.Writer, used, limit int64) {
	_, _ = fmt.Fprintf(w, "Error: embedding stopped: vaultmind reached %d MiB of memory, over its %d MiB limit (app.embed_max_rss_mb). "+
		"Any notes already embedded are kept; run the same command again to continue. "+
		"Embedding normally peaks near 3 GB, so please report this. 0 turns the check off.\n", used>>20, limit>>20)
	os.Exit(1)
}

// runEmbedGuarded runs an embed pass under the memory guard. Every embed pass
// in cmd goes through here (TestRunEmbed_OnlyThroughTheMemoryGuard).
func runEmbedGuarded(cmd *cobra.Command, idxr *index.Indexer, dbPath, model string, full bool) (*index.EmbedResult, error) {
	return guardEmbedMemory(cmd, func() (*index.EmbedResult, error) {
		return idxr.RunEmbed(cmd.Context(), dbPath, model, full)
	})
}

// guardEmbedMemory runs embed while watching this process's peak memory, and
// stops the process if it passes app.embed_max_rss_mb.
func guardEmbedMemory(cmd *cobra.Command, embed func() (*index.EmbedResult, error)) (*index.EmbedResult, error) {
	limit := int64(getConfigValueWithFlags[int](cmd, embedMaxRSSFlag, config.KeyAppEmbedMaxRssMb)) << 20
	stop := memguard.Watch(limit, embedGuardEvery, embedRSS, func(used int64) {
		exitOverMemory(cmd.ErrOrStderr(), used, limit)
	})
	defer stop()
	return embed()
}
