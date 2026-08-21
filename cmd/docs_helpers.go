// cmd/docs_helpers.go

package cmd

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/peiman/vaultmind/internal/marker"
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
	// A target that already carries a VAULTMIND:GENERATED:commands region is a
	// hand-written document embedding the catalog — replace only that region and
	// leave the prose around it alone. Without this, the guide an agent reads
	// first has to restate flag semantics the Cobra tree already owns, and prose
	// that duplicates a registry drifts from it: v0.7.0 shipped `--excerpt` with
	// the flag help as its only description and docs/AGENT_USAGE.md silent.
	//
	// Files with no markers keep the whole-file behaviour — that is what
	// internal/onboard/COMMANDS.md is, and must stay.
	if replaced, ok, err := injectIntoMarkedRegion(outputFile, md); err != nil {
		return err
	} else if ok {
		//nolint:gosec // G306: documentation file, readable by all is intended
		if err := os.WriteFile(outputFile, replaced, 0o644); err != nil {
			return fmt.Errorf("writing command reference into %s: %w", outputFile, err)
		}
		log.Info().Str("component", "docs").Str("file", outputFile).
			Str("section", docsCommandsSectionKey).Msg("Replaced generated command region")
		return nil
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

// docsCommandsSectionKey names the generated region inside hand-written guides.
const docsCommandsSectionKey = "commands"

// injectIntoMarkedRegion returns the file's bytes with the generated command
// region replaced. ok is false when the file does not exist or carries no such
// region, which tells the caller to fall back to writing the whole file.
//
// force is true: the region is owned by the generator, so a hand-edit inside it
// is exactly what this is meant to overwrite. The marker package's checksum
// guard protects human-owned regions; this one is not human-owned.
func injectIntoMarkedRegion(path, md string) ([]byte, bool, error) {
	// path is the operator's own --output target, which this same function then
	// writes to, so reading it first reaches nowhere the write does not already
	// reach. The suppression must sit on the line directly above the finding —
	// semgrep ignores it otherwise, which is how the first attempt at this
	// silently failed sast.
	// nosemgrep: go-path-traversal -- read of the caller's own output target
	raw, err := os.ReadFile(path) //nolint:gosec // G304: operator-supplied doc path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}
	markers, err := marker.FindMarkers(raw)
	if err != nil {
		return nil, false, fmt.Errorf("parsing markers in %s: %w", path, err)
	}
	found := false
	for i := range markers {
		if markers[i].SectionKey == docsCommandsSectionKey {
			found = true
			break
		}
	}
	if !found {
		return nil, false, nil
	}
	replaced, err := marker.ReplaceRegion(raw, docsCommandsSectionKey, []byte(md), true)
	if err != nil {
		return nil, false, fmt.Errorf("injecting command reference into %s: %w", path, err)
	}
	return replaced, true, nil
}
