// cmd/docs_helpers.go

package cmd

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// writeCommandsMarkdown writes the rendered command reference to outputFile, or
// to the command's output writer (stdout) when outputFile is empty. The file is
// written 0o644 — it is generated documentation committed to the repo, not a
// secret. A trailing newline is ensured so the committed file is POSIX-clean
// (and so the regenerate-and-diff drift gate is stable).
func writeCommandsMarkdown(cmd *cobra.Command, md, outputFile string) error {
	if outputFile == "" {
		_, err := fmt.Fprint(cmd.OutOrStdout(), md)
		return err
	}
	// 0644 is appropriate for committed, world-readable generated docs —
	// matches internal/docs (openOutputFile). The content is the public command
	// catalog, not a secret.
	//nolint:gosec // G306: documentation file, readable by all is intended
	if err := os.WriteFile(outputFile, []byte(md), 0o644); err != nil {
		return fmt.Errorf("writing command reference to %s: %w", outputFile, err)
	}
	log.Info().Str("component", "docs").Str("file", outputFile).Msg("Wrote command reference")
	return nil
}
