package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/spf13/cobra"
)

var importCmd = func() *cobra.Command {
	c := MustNewCommand(commands.ImportMetadata, runImport)
	c.Args = cobra.ExactArgs(1)
	return c
}()

func init() {
	// importdocs cannot import cmd (the command imports it). The version
	// the User-Agent names is the binary's, read when a request is made.
	importdocs.UserAgentVersion = Version
	MustAddToRoot(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	res, err := importDocs(cmd, args[0])
	if err != nil {
		return err
	}
	return writeImport(cmd, res)
}
