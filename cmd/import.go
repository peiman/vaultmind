package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var importCmd = func() *cobra.Command {
	c := MustNewCommand(commands.ImportMetadata, runImport)
	c.Args = cobra.ExactArgs(1)
	return c
}()

func init() {
	MustAddToRoot(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	res, err := importDocs(cmd, args[0])
	if err != nil {
		return err
	}
	return writeImport(cmd, res)
}
