package onboard_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The onboarding doc is embedded in the binary so a new user can read
// it without needing the source repo. The agent invokes
// `vaultmind init --print-instructions` and pipes the output to its
// own context. Tests pin: the embed exists, has substantive content,
// and contains the structural anchors that the doc must keep.

// TestInstructions_NotEmpty — the embed produced bytes. Catches the
// most basic regression where the embed directive silently fails or
// the source file disappears.
func TestInstructions_NotEmpty(t *testing.T) {
	doc := onboard.Instructions()
	require.NotEmpty(t, doc)
	require.Greater(t, len(doc), 1000,
		"onboarding doc should be substantive — under 1KB suggests the wrong file embedded")
}

// TestInstructions_ContainsStructuralAnchors — pin the section
// headers the agent relies on for navigation. If a future edit
// renames §1 Preflight or removes the migration path, this test
// surfaces it.
func TestInstructions_ContainsStructuralAnchors(t *testing.T) {
	doc := string(onboard.Instructions())
	required := []string{
		"# VaultMind — Agent Onboarding Guide",
		"## 1. Preflight",
		"## 2. Project read",
		"## 3. Branch decision",
		"## 4. Greenfield path",
		"## 5. Migration path",
		"## 6. Wire into Claude Code",
		"## 7. Diff-before-write protocol",
		"## 8. Verification checklist",
	}
	for _, anchor := range required {
		assert.Contains(t, doc, anchor,
			"onboarding doc must keep structural anchor %q", anchor)
	}
}

// TestPrintInstructions_WritesToWriter — `--print-instructions`
// composes via PrintInstructions(w). Pin the contract that it writes
// to the supplied writer (not stdout directly), so the cmd layer can
// route to test buffers and the agent can route to its own pipe.
func TestPrintInstructions_WritesToWriter(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, onboard.PrintInstructions(&buf))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# VaultMind"),
		"output should start with the doc's H1 — got: %q", out[:min(80, len(out))])
	assert.Equal(t, len(onboard.Instructions()), len(buf.Bytes()),
		"PrintInstructions should write the full embedded doc verbatim")
}
