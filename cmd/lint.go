package cmd

import "github.com/spf13/cobra"

// lintCmd is the DEPRECATED top-level `lint` parent. Wikilink repair moved
// under the doctor health hub (`doctor heal wikilinks`); `dataview lint` is a
// separate domain checker and is unaffected. The parent is hidden from the
// root listing; its `fix-links` subcommand survives as a hidden deprecated
// alias that delegates to the new `doctor heal wikilinks` path. Kept for ~2
// releases.
var lintCmd = &cobra.Command{
	Use:    "lint",
	Short:  "Deprecated: use 'doctor heal wikilinks'",
	Hidden: true,
}

func init() {
	RootCmd.AddCommand(lintCmd)
}
