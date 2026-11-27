package cmd

import (
	"encoding/json"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

// This helper owns arc-recitation config reads; keep the command metadata
// reference here so the namespace test can attribute those reads explicitly.
var _ = commands.ArcReciteMetadata

func arcReciteConfig(cmd *cobra.Command) (memory.ReciteConfig, error) {
	return memory.ParseReciteConfig(
		getConfigValueWithFlags[string](cmd, "type", config.KeyAppArcReciteType),
		getConfigValueWithFlags[int](cmd, "budget", config.KeyAppArcReciteBudget),
		getConfigValueWithFlags[int](cmd, "excerpt", config.KeyAppArcReciteExcerpt),
		retrievalIncludeDrafts(cmd),
		getConfigValueWithFlags[string](cmd, "sections", config.KeyAppArcReciteSections),
		getConfigValueWithFlags[string](cmd, "full", config.KeyAppArcReciteFull),
		getConfigValueWithFlags[string](cmd, "shard", config.KeyAppArcReciteShard),
		getConfigValueWithFlags[int](cmd, "max-bytes", config.KeyAppArcReciteMaxBytes),
	)
}

func recordArcReciteAccess(cmd *cobra.Command, vaultPath string, result *memory.ReciteResult) {
	// Record the read. Every item here had its body delivered, so leaving no
	// trace would make `arc recite` exactly the untracked bulk path that issue
	// #53 exists to replace — 27 bodies reaching the agent and a ledger that
	// says nothing happened.
	//
	// AccessSourceRecite is deliberately NOT an activation signal: it fires for
	// the whole layer every session, so boosting on it would add the same
	// constant to every arc and flatten the ranking it feeds. Honest in the
	// ledger, inert in the scorer. Best-effort — telemetry never fails a read.
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		for _, item := range result.Items {
			_, _ = session.LogNoteAccessEventAs(item.ID, experiment.AccessSourceRecite, item.Excerpt != "", experiment.CallerRecite)
		}
	}
}

func renderArcRecite(cmd *cobra.Command, result *memory.ReciteResult) error {
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcReciteJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("arc recite", result))
	}
	return writeRecitation(cmd.OutOrStdout(), result)
}
