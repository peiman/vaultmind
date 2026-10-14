package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// retrievalIncludeDrafts reads the one shared retrieval switch. Every command
// that ranks or delivers notes calls it. The key lives under app.retrieval,
// so this file is the owner the namespace check allows (ADR-102, Principle 7).
func retrievalIncludeDrafts(cmd *cobra.Command) bool {
	return getConfigValueWithFlags[bool](cmd, "include-drafts", config.KeyAppRetrievalIncludeDrafts)
}

// registerIncludeDrafts adds --include-drafts. The key is not under a command
// prefix, so MustNewCommand does not register it, and a deprecated alias does
// not copy a flag registered by hand on the command it delegates to.
func registerIncludeDrafts(cmd *cobra.Command) {
	cmd.Flags().Bool("include-drafts", false, commands.IncludeDraftsDescription)
}
