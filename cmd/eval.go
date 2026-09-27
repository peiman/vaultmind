package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var evalCmd = func() *cobra.Command {
	c := MustNewCommand(commands.EvalMetadata, runEval)
	c.Args = cobra.ExactArgs(1)
	return c
}()

func init() {
	MustAddToRoot(evalCmd)
}

func runEval(cmd *cobra.Command, args []string) error {
	res, err := evaluateVault(cmd, args[0])
	if err != nil {
		return err
	}
	return writeEval(cmd, res)
}
