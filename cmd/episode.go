// ckeletin:allow-custom-command
//
// `episode` is a utility command with no persistent user-facing config keys:
// its single flag (--output-dir) defaults to a path inside the project's
// identity vault, and its positional arg is a path provided per-invocation.
// There are no viper-bound settings worth the ceremony of the ckeletin
// config registry + generated constants. The marker above documents the
// deliberate exception to the MustNewCommand pattern used by config-driven
// commands elsewhere in cmd/.
package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/spf13/cobra"
)

var episodeCmd = &cobra.Command{
	Use:   "episode",
	Short: "Session-episode capture (episodic-memory substrate, v0)",
}

var episodeCaptureCmd = &cobra.Command{
	Use:   "capture <transcript-path>",
	Short: "Parse a Claude Code JSONL transcript and write a markdown episode",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir, _ := cmd.Flags().GetString("output-dir")
		path, err := episode.Capture(args[0], outputDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	},
}

func init() {
	episodeCaptureCmd.Flags().String("output-dir", "vaultmind-identity/episodes", "Directory to write the episode markdown file")
	episodeCmd.AddCommand(episodeCaptureCmd)
	MustAddToRoot(episodeCmd)
}
