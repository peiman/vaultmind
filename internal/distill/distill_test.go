package distill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rendered episode .md, matching internal/episode's render format: verbatim
// user + assistant sections with "### N — timestamp" turn headers.
const sampleEpisode = `---
id: episode-2026-05-31-abcd1234
type: episode
session_id: abcd1234
---

# Episode — episode-2026-05-31-abcd1234

## Metadata

- User messages: 4

## User messages (verbatim)

### 1 — 2026-05-31T10:00:00.000Z

> can you refactor the parser

### 2 — 2026-05-31T10:05:00.000Z

> regarding the subagent. just make sure it fixes it, you have full autonomy there. dont need to ask me.

### 3 — 2026-05-31T10:10:00.000Z

> fix and commit atomically make sure you have the manifesto lens on

### 4 — 2026-05-31T10:15:00.000Z

> go for it

### 5 — 2026-05-31T10:20:00.000Z

> This session is being continued from a previous conversation that ran out of context. It covered the manifesto lens and you have full autonomy.

## Assistant responses (verbatim)

### 1 — 2026-05-31T10:00:30.000Z

I committed and task check passes.

## Principle

Enforce only what the system reads.

### deeper note — a subheading in the body

Done.
`

func writeSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "episode-2026-05-31-abcd1234.md")
	require.NoError(t, os.WriteFile(p, []byte(sampleEpisode), 0o600))
	return p
}

func TestParseEpisodeFile_ExtractsVerbatimTurns(t *testing.T) {
	ep, err := distill.ParseEpisodeFile(writeSample(t))
	require.NoError(t, err)
	assert.Equal(t, "episode-2026-05-31-abcd1234", ep.ID)
	require.Len(t, ep.UserTurns, 5, "five verbatim user turns")
	assert.Equal(t, 1, ep.UserTurns[0].Index)
	assert.Equal(t, "2026-05-31T10:00:00.000Z", ep.UserTurns[0].Timestamp)
	assert.Equal(t, "can you refactor the parser", ep.UserTurns[0].Text,
		"the '> ' blockquote prefix is stripped to clean verbatim")
	assert.Contains(t, ep.UserTurns[1].Text, "full autonomy")

	// C1 regression: assistant turns render raw (un-blockquoted) and routinely
	// contain markdown headings — these must stay in the turn body, NOT truncate
	// it or fabricate spurious turns. One assistant turn, full body preserved.
	require.Len(t, ep.AssistantTurns, 1, "the '## Principle' / '### note' headings must NOT split the assistant turn")
	body := ep.AssistantTurns[0].Text
	assert.Contains(t, body, "task check passes")
	assert.Contains(t, body, "## Principle", "a markdown heading in assistant prose is body, not structure")
	assert.Contains(t, body, "### deeper note", "a non-turn-header '###' line stays in the body")
	assert.Contains(t, body, "Done.", "content after the headings is not dropped")
}

// ExtractCandidates fires the two high-precision mechanical rules and nothing
// else: the authority-grant (turn 2) and the manifesto-lens (turn 3). The bare
// approval "go for it" (turn 4) and the plain request (turn 1) must NOT fire.
func TestExtractCandidates_FiresRules1And3Only(t *testing.T) {
	ep, err := distill.ParseEpisodeFile(writeSample(t))
	require.NoError(t, err)
	cands := distill.ExtractCandidates(ep)

	byRule := map[distill.RuleID][]distill.Candidate{}
	for _, c := range cands {
		byRule[c.Rule] = append(byRule[c.Rule], c)
	}
	require.Len(t, byRule[distill.RuleAuthorityGrant], 1, "the autonomy-transfer grant fires")
	assert.Equal(t, 2, byRule[distill.RuleAuthorityGrant][0].TurnIndex)
	assert.Contains(t, byRule[distill.RuleAuthorityGrant][0].Verbatim, "dont need to ask me")

	require.Len(t, byRule[distill.RuleManifestoLens], 1, "the manifesto-lens fires")
	assert.Equal(t, 3, byRule[distill.RuleManifestoLens][0].TurnIndex)

	// Nothing else: 2 candidates total. "go for it" (bare approval, turn 4), the
	// plain request (turn 1), and the COMPACTION SUMMARY (turn 5 — contains both
	// "manifesto lens" and "full autonomy" but is machine-injected, not a push)
	// must all NOT fire.
	assert.Len(t, cands, 2, "bare approval, plain request, and compaction summary must not fire")
}

// A compaction-summary turn is the dominant real-corpus false positive: it
// contains trigger phrases (it summarizes a prior session) but is machine-
// injected, not a partner push. It must be skipped.
func TestExtractCandidates_SkipsCompactionSummary(t *testing.T) {
	ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{
		{Index: 1, Text: "This session is being continued from a previous conversation that ran out of context. It discussed the manifesto lens and you have full autonomy."},
	}}
	assert.Empty(t, distill.ExtractCandidates(ep), "compaction summary must not produce candidates")
}

// Rule 3 is tightened: a permission-style approval token alone ("go for it",
// "yes please", "ok") is a task-approval, not a standing autonomy transfer.
func TestExtractCandidates_RejectsBareApproval(t *testing.T) {
	for _, msg := range []string{"go for it", "yes please", "ok do it", "sounds good", "Push"} {
		ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{{Index: 1, Text: msg}}}
		assert.Empty(t, distill.ExtractCandidates(ep), "bare approval %q must not be an authority-grant", msg)
	}
}

// The autonomy-transfer lexemes that DO fire (the standing "you decide" shift).
func TestExtractCandidates_AuthorityGrantLexemes(t *testing.T) {
	for _, msg := range []string{
		"you have full autonomy there",
		"you decide, you use it",
		"dont need to ask me",
		"I trust you, fix all",
		"do it as you see fit",
		"you are the one who should evaluate it because you are using vaultmind not me",
	} {
		ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{{Index: 1, Text: msg}}}
		cands := distill.ExtractCandidates(ep)
		require.Len(t, cands, 1, "autonomy-transfer %q must fire", msg)
		assert.Equal(t, distill.RuleAuthorityGrant, cands[0].Rule)
	}
}

// The evidence-gate rule fires when the partner makes proceeding conditional on
// the agent's own confidence — the arc shape the detector missed in the Siavoush
// content-machine field report (an "if you are confident we merge" arc that no
// authority-grant phrase caught).
func TestExtractCandidates_EvidenceGateFires(t *testing.T) {
	for _, msg := range []string{
		"if you are confident we merge",
		"if you're confident, ship it",
		"only if you are sure — otherwise ask me",
		"as long as you're sure, go ahead and refactor",
	} {
		ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{{Index: 1, Text: msg}}}
		cands := distill.ExtractCandidates(ep)
		require.Len(t, cands, 1, "confidence-conditional delegation %q must fire", msg)
		assert.Equal(t, distill.RuleEvidenceGate, cands[0].Rule)
	}
}

// Precision guard: bare confidence/sureness prose is NOT a delegation gate —
// only the "if you're <confident>" construction is. Keeps the rule as tight as
// the others (the 2026-05-31 review's precision bar).
func TestExtractCandidates_EvidenceGateRejectsBareConfidence(t *testing.T) {
	for _, msg := range []string{
		"are you sure about this?",
		"I'm confident this is right",
		"make sure the tests pass",
	} {
		ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{{Index: 1, Text: msg}}}
		assert.Empty(t, distill.ExtractCandidates(ep), "bare confidence prose %q must not fire", msg)
	}
}

// The manifesto-lens rule also fires on a numbered-principle citation
// ("principle 7"), via principleNRe — but not on un-numbered "principled" prose.
func TestExtractCandidates_PrincipleNFiresManifestoLens(t *testing.T) {
	ep := &distill.Episode{ID: "e", UserTurns: []distill.Turn{
		{Index: 1, Text: "apply principle 7 here"},
		{Index: 2, Text: "a principled approach with no number"},
	}}
	cands := distill.ExtractCandidates(ep)
	require.Len(t, cands, 1, "only the numbered-principle turn fires")
	assert.Equal(t, distill.RuleManifestoLens, cands[0].Rule)
	assert.Equal(t, "principle 7", cands[0].Trigger)
	assert.Equal(t, 1, cands[0].TurnIndex)
}

// An episode with no user section parses cleanly (no panic, no user turns) and
// yields no candidates.
func TestParseEpisodeFile_NoUserSection(t *testing.T) {
	md := "---\nid: episode-x\n---\n\n## Assistant responses (verbatim)\n\n### 1 — 2026-05-31T10:00:00.000Z\n\nhello\n"
	dir := t.TempDir()
	p := filepath.Join(dir, "episode-x.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o600))

	ep, err := distill.ParseEpisodeFile(p)
	require.NoError(t, err)
	assert.Equal(t, "episode-x", ep.ID)
	assert.Empty(t, ep.UserTurns)
	require.Len(t, ep.AssistantTurns, 1)
	assert.Equal(t, "hello", ep.AssistantTurns[0].Text)
	assert.Empty(t, distill.ExtractCandidates(ep), "no user turns → no candidates, no panic")
}

// ScanEpisodes globs + filters + parses a directory and aggregates candidates,
// recording (not swallowing) per-episode parse errors.
func TestScanEpisodes(t *testing.T) {
	dir := t.TempDir()
	padded := sampleEpisode + strings.Repeat("\n> filler line to clear the signal floor", 200)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.md"), []byte(padded), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noise.md"), make([]byte, 1000), 0o600))

	r, err := distill.ScanEpisodes(dir)
	require.NoError(t, err)
	assert.Equal(t, 2, r.EpisodesScanned)
	assert.Equal(t, 1, r.EpisodesKept, "the sub-threshold noise episode is filtered")
	assert.Len(t, r.Candidates, 2, "the signal episode's authority-grant + manifesto-lens surface")
	assert.Empty(t, r.ParseErrors)
}

func TestSignalFilter_DropsNoiseEpisodes(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.md")
	small := filepath.Join(dir, "small.md")
	require.NoError(t, os.WriteFile(big, make([]byte, 20_000), 0o600))
	require.NoError(t, os.WriteFile(small, make([]byte, 3_000), 0o600))
	gone := filepath.Join(dir, "gone.md") // can't be stat'd

	kept := distill.SignalFilter([]string{big, small, gone}, distill.MinEpisodeBytes)
	require.Len(t, kept, 2, "only the CONFIRMED-small episode is dropped")
	assert.Contains(t, kept, big)
	assert.Contains(t, kept, gone, "an un-stat-able path is KEPT, not silently dropped (H1)")
	assert.NotContains(t, kept, small)
}
