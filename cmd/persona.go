// ckeletin:allow-custom-command - grouping parent with no leaf configuration
package cmd

import "github.com/spf13/cobra"

// personaCmd groups measurements about persona reconstruction.
var personaCmd = &cobra.Command{
	Use:   "persona",
	Short: "Measure persona reconstruction",
}

func init() {
	RootCmd.AddCommand(personaCmd)
}
