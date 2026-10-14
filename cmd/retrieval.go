package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/spf13/cobra"
)

// retrievalIncludeDrafts reads the one shared retrieval switch. Ask and search
// both call it. The key lives under app.retrieval, not under either command,
// so this file is the owner the namespace check allows (ADR-102, Principle 7).
func retrievalIncludeDrafts(cmd *cobra.Command) bool {
	return getConfigValueWithFlags[bool](cmd, "include-drafts", config.KeyAppRetrievalIncludeDrafts)
}
