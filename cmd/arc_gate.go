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
	arm := getConfigValueWithFlags[bool](cmd, "arm", config.KeyAppArcGateArm)
	home := os.Getenv("HOME")
	stateDir, armedDir := arcGateStateDirs()
	logDir := ""
	if arm {
		runArcGateArm(cmd, input, armedDir)
		return nil
	}
	if home != "" {
		logDir = filepath.Join(home, ".vaultmind", "persona-eval")
	}
	requireArm := os.Getenv("VAULTMIND_ARC_GATE_REQUIRE_ARM") == "1"
	stdout, stderr := arcgate.GateWithArmRequirement(input, vault, event, os.Getenv("VAULTMIND_ARC_TRIGGERS"), stateDir, armedDir, logDir, time.Now().UTC(), requireArm)
	if stderr != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), stderr)
		return nil
	}
	_, _ = io.WriteString(cmd.OutOrStdout(), stdout)
	return nil
}

func arcGateStateDirs() (stateDir, armedDir string) {
	dir, err := xdg.StateDir()
	if err != nil {
		return "", ""
	}
	stateDir = filepath.Join(dir, "arc-gate")
	return stateDir, filepath.Join(filepath.Dir(stateDir), "arc-gate-armed")
}

func runArcGateArm(cmd *cobra.Command, input []byte, armedDir string) {
	if err := arcgate.Arm(input, armedDir); err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "arc gate: "+err.Error())
	}
}
