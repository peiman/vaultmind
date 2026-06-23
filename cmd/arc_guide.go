// cmd/arc_guide.go
// ckeletin:allow-custom-command

package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/spf13/cobra"
)

// arcGuideCmd prints the canonical arc-writing discipline (distill.ArcGuide).
// It takes no vault and no flags — first contact must be self-serve, so an
// adopting agent can learn how to find and write its own arcs with zero human
// (principle-ax-design: "make first contact self-serve"; "document the loop, not
// just the verbs"). A later slice will reconcile init's separate how-to-write-arcs
// template to distill.ArcGuide so a new vault is seeded with this same text.
var arcGuideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Print the arc-writing discipline — how to find and write your own arcs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), distill.ArcGuide)
		return err
	},
}

func init() {
	arcCmd.AddCommand(arcGuideCmd)
}
