package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/spf13/cobra"
)

// newDeprecatedAlias builds a Hidden cobra command that prints a one-line
// stderr deprecation notice, then delegates to the new command's run path by
// calling delegate with its own (alias) command and args. Because the alias
// registers the same flags as the target (via meta.ConfigPrefix), delegate —
// which is the target's run function — reads flags transparently from the
// alias command.
//
// This helper is shared by every slice of the taxonomy refactor (links→memory,
// doctor, lint→doctor heal); keep it general.
func newDeprecatedAlias(meta config.CommandMetadata, notice string, delegate func(*cobra.Command, []string) error) *cobra.Command {
	cmd := MustNewCommand(meta, func(c *cobra.Command, args []string) error {
		if _, err := fmt.Fprintln(c.ErrOrStderr(), notice); err != nil {
			return err
		}
		return delegate(c, args)
	})
	cmd.Hidden = true
	return cmd
}
