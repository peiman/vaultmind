package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/spf13/cobra"
)

var initCmd = func() *cobra.Command {
	c := MustNewCommand(commands.InitMetadata, runInit)
	c.Args = cobra.ExactArgs(1)
	return c
}()

func init() {
	MustAddToRoot(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	res, err := initvault.Init(args[0])
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "✅ Vault scaffolded at %s (%d files)\n\n", res.VaultPath, res.FilesAdded)
	_, _ = fmt.Fprintf(w, "Next steps:\n")
	_, _ = fmt.Fprintf(w, "  cd %s\n", res.VaultPath)
	_, _ = fmt.Fprintf(w, "  vaultmind index --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind index --embed --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind ask \"who am I\" --vault .\n\n")
	_, _ = fmt.Fprintf(w, "Edit identity/who-am-i.md and references/current-context.md to make it yours.\n")
	return nil
}
