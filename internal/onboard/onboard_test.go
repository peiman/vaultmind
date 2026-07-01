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

// TestOnboarding_PointsToIssuesFeedback — both the default quick-start and the
// full guide must tell the adopting agent to report findings as vaultmind-oss
// issues (Peiman 2026-06-23). The consumer-evidence loop only closes if a
// self-serve adopter (no mesh link) is handed an explicit channel.
func TestOnboarding_PointsToIssuesFeedback(t *testing.T) {
	const issuesURL = "github.com/peiman/vaultmind/issues"
	assert.Contains(t, string(onboard.QuickStart()), issuesURL,
		"quick-start must name the issues feedback channel")
	assert.Contains(t, string(onboard.Instructions()), issuesURL,
		"full guide must name the issues feedback channel")
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

// TestCommands_NotEmptyAndIsGroupedReference — the COMMANDS.md embed produced
// bytes and carries the generated reference's structural anchors. Catches a
// silently-failing embed or the wrong file.
func TestCommands_NotEmptyAndIsGroupedReference(t *testing.T) {
	ref := onboard.Commands()
	require.NotEmpty(t, ref)
	require.Greater(t, len(ref), 500,
		"command reference should be substantive — under 500 bytes suggests the wrong file embedded")
	doc := string(ref)
	assert.True(t, strings.HasPrefix(doc, "# VaultMind Commands"),
		"embedded reference should start with the generated H1")
	assert.Contains(t, doc, "| Command | What | When to use |",
		"embedded reference should be the grouped markdown table")
}

// TestPrintFull_AppendsCommandReference — `--full` composes via PrintFull(w),
// which writes the full onboarding guide AND the command reference. Pin both
// halves are present and ordered (guide first, reference second).
func TestPrintFull_AppendsCommandReference(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, onboard.PrintFull(&buf))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# VaultMind"),
		"output should start with the onboarding guide's H1")
	assert.Contains(t, out, "# VaultMind Commands",
		"PrintFull must append the command reference")
	// Guide precedes the appended reference.
	assert.Less(t, strings.Index(out, "## 1. Preflight"),
		strings.Index(out, "# VaultMind Commands"),
		"the command reference must come after the onboarding guide")
	// Both embedded bodies are present verbatim.
	assert.Contains(t, out, string(onboard.Instructions()))
	assert.Contains(t, out, string(onboard.Commands()))
}

// failAfterWriter writes successfully `ok` times, then returns an error on the
// next Write — lets a test target each of PrintFull's three Write calls (guide,
// separator, command reference) and assert the wrapped error surfaces.
type failAfterWriter struct {
	ok  int
	err error
}

func (f *failAfterWriter) Write(p []byte) (int, error) {
	if f.ok <= 0 {
		return 0, f.err
	}
	f.ok--
	return len(p), nil
}

// TestPrintFull_PropagatesWriteErrors — each of the three writes in PrintFull
// (instructions, separator, command reference) wraps and returns its writer
// error, so a broken stdout/pipe is reported rather than swallowed.
func TestPrintFull_PropagatesWriteErrors(t *testing.T) {
	sentinel := assert.AnError
	cases := []struct {
		name    string
		okFirst int
		wantMsg string
	}{
		{"instructions write fails", 0, "writing onboarding instructions"},
		{"separator write fails", 1, "writing onboarding separator"},
		{"command reference write fails", 2, "writing command reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := onboard.PrintFull(&failAfterWriter{ok: tc.okFirst, err: sentinel})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.ErrorIs(t, err, sentinel)
		})
	}
}

// TestQuickStart_NotEmptyAndConcise — the quick-start embed produced bytes
// AND is substantially smaller than the full guide. The whole point of the
// quick-start (slice #4) is the skimmable 20%; if it grows to monolith size
// it has failed its purpose, so assert it stays under half the full doc.
func TestQuickStart_NotEmptyAndConcise(t *testing.T) {
	qs := onboard.QuickStart()
	require.NotEmpty(t, qs)
	require.Greater(t, len(qs), 200,
		"quick-start should be substantive — under 200 bytes suggests the wrong file embedded")
	full := onboard.Instructions()
	assert.Less(t, len(qs), len(full)/2,
		"quick-start must be substantially smaller than the full guide (the concise 20%)")
}

// TestPrintQuickStart_WritesToWriter — `--print-instructions` (default)
// composes via PrintQuickStart(w). Pin that it writes the quick-start bytes
// verbatim to the supplied writer so the cmd layer can route to a buffer.
func TestPrintQuickStart_WritesToWriter(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, onboard.PrintQuickStart(&buf))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# VaultMind"),
		"output should start with the quick-start's H1 — got: %q", out[:min(80, len(out))])
	assert.Equal(t, len(onboard.QuickStart()), len(buf.Bytes()),
		"PrintQuickStart should write the full embedded quick-start verbatim")
}

// meshQuickStartSizeCeiling caps the embedded mesh quick-start. The doc's whole
// purpose is the concise "how the verbs fit together" narrative; if it grows
// past ~4 KB it has drifted into a manual, so the gate forces a re-think.
const meshQuickStartSizeCeiling = 4096

// TestMeshQuickStart_NotEmpty — the mesh quick-start embed produced bytes.
// Catches a silently-failing embed directive or the wrong/missing source file.
func TestMeshQuickStart_NotEmpty(t *testing.T) {
	qs := onboard.MeshQuickStart()
	require.NotEmpty(t, qs)
	require.Greater(t, len(qs), 500,
		"mesh quick-start should be substantive — under 500 bytes suggests the wrong file embedded")
}

// TestMeshQuickStart_ContainsStructuralAnchors — pin the section headers the
// admin + member journeys rely on. If a future edit renames a journey or drops
// the liveness section, this surfaces it.
func TestMeshQuickStart_ContainsStructuralAnchors(t *testing.T) {
	doc := string(onboard.MeshQuickStart())
	required := []string{
		"# VaultMind — Mesh Onboarding (Contract-B)",
		"## Admin",
		"## Member",
		"## Am I live?",
	}
	for _, anchor := range required {
		assert.Contains(t, doc, anchor,
			"mesh quick-start must keep structural anchor %q", anchor)
	}
}

// TestMeshQuickStart_StaysConcise — the doc must stay under the size ceiling so
// it remains the skimmable narrative, not a manual. Mirrors the QuickStart
// size-gate intent.
func TestMeshQuickStart_StaysConcise(t *testing.T) {
	assert.Less(t, len(onboard.MeshQuickStart()), meshQuickStartSizeCeiling,
		"mesh quick-start must stay under %d bytes (the concise narrative)", meshQuickStartSizeCeiling)
}

// TestPrintMeshQuickStart_WritesToWriter — `identity --print-instructions`
// composes via PrintMeshQuickStart(w). Pin that it writes the mesh quick-start
// bytes verbatim to the supplied writer so the cmd layer can route to a buffer.
func TestPrintMeshQuickStart_WritesToWriter(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, onboard.PrintMeshQuickStart(&buf))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# VaultMind"),
		"output should start with the mesh quick-start's H1 — got: %q", out[:min(80, len(out))])
	assert.Equal(t, len(onboard.MeshQuickStart()), len(buf.Bytes()),
		"PrintMeshQuickStart should write the full embedded mesh quick-start verbatim")
}
