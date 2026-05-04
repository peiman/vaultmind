// Package onboard provides the embedded agent-onboarding script as a
// CLI-accessible asset. The canonical doc lives in this package
// (AGENT_ONBOARDING.md) and is embedded in the binary at build time;
// `vaultmind init --print-instructions` prints it on demand. Embedding
// rather than reading from disk means the doc travels with the binary
// — a new user with `go install`'d vaultmind can read the script
// without needing to clone the source repo.
//
// Source-of-truth lives here, not under docs/, because Go's embed
// directive cannot traverse parent directories. README links to this
// path as the human-discoverable copy; the binary embeds the same
// file, so there is one source of truth (manifesto principle 7).
package onboard

import (
	_ "embed"
	"fmt"
	"io"
)

//go:embed AGENT_ONBOARDING.md
var instructions []byte

// Instructions returns the embedded agent-onboarding doc as raw
// bytes. Returns a copy-safe slice — callers should not mutate.
func Instructions() []byte {
	return instructions
}

// PrintInstructions writes the embedded onboarding doc verbatim to
// the supplied writer. Used by `vaultmind init --print-instructions`
// to route output to stdout (or a test buffer). Returns the writer's
// error if any; the embed itself cannot fail at runtime.
func PrintInstructions(w io.Writer) error {
	if _, err := w.Write(instructions); err != nil {
		return fmt.Errorf("writing onboarding instructions: %w", err)
	}
	return nil
}
