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
		return onboard.PrintInstructions(cmd.OutOrStdout())
	}
	if len(args) != 1 {
		return fmt.Errorf("vaultmind init requires a vault path (or use --print-instructions)")
	}
	return runInitScaffold(cmd, args[0])
}
