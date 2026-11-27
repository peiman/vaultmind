package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/arcgate"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

var arcGateCmd = MustNewCommand(commands.ArcGateMetadata, runArcGate)

func init() {
	arcCmd.AddCommand(arcGateCmd)
}

func runArcGate(cmd *cobra.Command, _ []string) error {
	input, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "arc gate: cannot read hook stdin:", err)
		return nil
	}
	vault := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcGateVault)
	event := getConfigValueWithFlags[string](cmd, "event", config.KeyAppArcGateEvent)
	home := os.Getenv("HOME")
	stateDir, logDir := "", ""
	if dir, err := xdg.StateDir(); err == nil {
		stateDir = filepath.Join(dir, "arc-gate")
	}
	if home != "" {
		logDir = filepath.Join(home, ".vaultmind", "persona-eval")
	}
	stdout, stderr := arcgate.Gate(input, vault, event, os.Getenv("VAULTMIND_ARC_TRIGGERS"), stateDir, logDir, time.Now().UTC())
	if stderr != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), stderr)
		return nil
	}
	_, _ = io.WriteString(cmd.OutOrStdout(), stdout)
	return nil
}
