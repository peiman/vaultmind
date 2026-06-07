package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/spf13/cobra"
)

var initCmd = func() *cobra.Command {
	c := MustNewCommand(commands.InitMetadata, runInit)
	// path is required EXCEPT when --print-instructions is set.
	c.Args = cobra.MaximumNArgs(1)
	return c
}()

func init() {
	MustAddToRoot(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if getConfigValueWithFlags[bool](cmd, "print-instructions", config.KeyAppInitPrintInstructions) {
		return printOnboarding(cmd)
	}
	if len(args) != 1 {
		return fmt.Errorf("vaultmind init requires a vault path (or use --print-instructions)")
	}
	return runInitScaffold(cmd, args[0], initWireParams{
		wireHooks:  getConfigValueWithFlags[bool](cmd, "wire-hooks", config.KeyAppInitWireHooks),
		local:      getConfigValueWithFlags[bool](cmd, "local", config.KeyAppInitLocal),
		dryRun:     getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppInitDryRun),
		projectDir: getConfigValueWithFlags[string](cmd, "project-dir", config.KeyAppInitProjectDir),
	})
}

// printOnboarding emits the concise quick-start by default, or the full
// agent-onboarding guide plus the generated grouped command reference when
// --full is set. Both write to the command's output writer so tests can route
// to a buffer.
func printOnboarding(cmd *cobra.Command) error {
	if getConfigValueWithFlags[bool](cmd, "full", config.KeyAppInitFull) {
		return onboard.PrintFull(cmd.OutOrStdout())
	}
	return onboard.PrintQuickStart(cmd.OutOrStdout())
}
