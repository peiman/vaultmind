package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SelfMetadata defines the metadata for the self command — the agent's
// own memory-state introspection view (recent / hot / stale).
var SelfMetadata = config.CommandMetadata{
	Use:   "self",
	Short: "Show your memory state — recent, hot, stale notes",
	Long: `Render the activation state of your vault — first-person AX for the agent
using vaultmind as long-term memory. Reads access_count + last_accessed_at,
which RecordNoteAccess populates on every successful Ask + note get.

THREE SECTIONS

  Recent (newest first):     The last N notes you touched. Anchors you in
                             "where am I right now" in conceptual space.
  Hot (top activation):      Ranked by ln(1+count) - d*ln(elapsed_hours).
                             Captures both frequency and recency.
  Stale (drifting away):     Accessed but past the threshold. What your
                             memory used to hold but isn't returning to.

The hot column shows activation as +0.00 for the leader, then negative
numbers indicating how-much-below the leader. Order-preserving — the
sign is just relative-distance, not "broken".

EXAMPLES

  vaultmind self --vault vaultmind-vault
      Default — limit 10 per section, 7-day stale threshold.

  vaultmind self --vault vaultmind-identity --limit 5
      Tighter view — 5 rows per section.

THIS IS ALREADY AUTO-INJECTED

  The SessionStart hook runs 'vaultmind self' for both identity and research
  vaults at session start. You see your memory state ambiently before any
  work begins. Run this command manually only when you want a fresh check
  mid-session.`,
	ConfigPrefix: "app.self",
	FlagOverrides: map[string]string{
		"app.self.vault": "vault",
		"app.self.limit": "limit",
	},
}

// SelfOptions returns configuration options for the self command.
func SelfOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.self.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.self.limit", DefaultValue: 10, Description: "Max rows per section (recent/hot/stale)", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(SelfOptions)
}
