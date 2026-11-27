package cmd

import (
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var arcReciteCmd = MustNewCommand(commands.ArcReciteMetadata, runArcRecite)

func init() {
	arcCmd.AddCommand(arcReciteCmd)
	registerIncludeDrafts(arcReciteCmd)
}

// runArcRecite loads the whole arc layer as bodies.
//
// The counterpart to `ask`: that command ranks, this one enumerates. See
// commands.ArcReciteMetadata for why an identity layer must not be retrieved.
func runArcRecite(cmd *cobra.Command, _ []string) error {
	reciteConfig, err := arcReciteConfig(cmd)
	if err != nil {
		return err
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcReciteVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "arc recite")
	if err != nil {
		return err
	}
	defer vdb.Close()

	result, err := memory.Recite(vdb.DB, reciteConfig)
	if err != nil {
		return err
	}

	recordArcReciteAccess(cmd, vaultPath, result)
	return renderArcRecite(cmd, result)
}

// writeRecitation renders the layer, and says plainly what did not fit.
//
// The imperative frame is deliberate. These are not search results to consider
// — they are the agent's own transformations, and the header says so, because
// an identity layer delivered as a neutral list reads as background and gets
// skipped. That skipping is the failure #47 documents.
func writeRecitation(w io.Writer, r *memory.ReciteResult) error {
	_, err := io.WriteString(w, memory.RenderRecitation(r))
	return err
}
