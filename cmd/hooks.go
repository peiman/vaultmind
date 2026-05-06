package cmd

import "github.com/spf13/cobra"

// hooksCmd is the parent group for hook-related subcommands. Subs
// register themselves via init() in their own files (cmd/hooks_install.go
// for `vaultmind hooks install`).
var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage VaultMind's Claude Code hook scripts",
	Long: `Manage Claude Code hook scripts that wire VaultMind into a project.

The canonical hook scripts live embedded in the vaultmind binary
(internal/hookscripts/). Subcommands here write them out, check
for drift, and report status.

Subcommands:
  install   Write embedded hook scripts into a project's .claude/scripts/`,
}

func init() {
	RootCmd.AddCommand(hooksCmd)
}
