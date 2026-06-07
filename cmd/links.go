package cmd

import "github.com/spf13/cobra"

// linksCmd is the DEPRECATED top-level `links` parent. Graph traversal moved
// under `memory` (memory links / memory neighbors). The parent is hidden from
// the root listing; its subcommands remain as hidden deprecated aliases that
// delegate to the new `memory` paths. Kept for ~2 releases.
var linksCmd = &cobra.Command{
	Use:    "links",
	Short:  "Deprecated: use 'memory links' / 'memory neighbors'",
	Hidden: true,
}

func init() {
	RootCmd.AddCommand(linksCmd)
}
