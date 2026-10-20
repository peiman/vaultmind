package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctor_ReportsPinnedCountAndCost(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_NO_UPDATE_CHECK", "1")
	t.Setenv("VAULTMIND_NO_SERVE", "1")
	body := "short ledger"
	dir := buildPinnedVault(t, map[string]string{
		"ledger.md": pinnedNote("work-ledger", "Work Ledger", true, body),
	})

	out, _, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err)
	text := out.String()
	tokens := memory.EstimateTokens(body)
	budget := strconv.Itoa(memory.SessionPinnedBudget)
	assert.Contains(t, text, "Pinned notes: 1 ("+itoa(tokens)+" tokens at session start, "+itoa(tokens)+" in full; default budget "+budget+")")
	assert.Contains(t, text, "work-ledger")
	assert.NotContains(t, text, "Work Ledger", "a title is free text; doctor lists the id and the cost")
	assert.NotContains(t, text, "unpin")

	jout, _, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err)
	pinned := decodeDoctorPinned(t, jout.Bytes())
	assert.Equal(t, 1, pinned.Count)
	assert.Equal(t, tokens, pinned.Tokens)
	assert.Equal(t, tokens, pinned.SessionTokens)
	assert.Equal(t, []string{}, pinned.Omitted)
	assert.Equal(t, memory.SessionPinnedBudget, pinned.Budget)
	assert.False(t, pinned.OverBudget)
	require.Len(t, pinned.Notes, 1)
	assert.Equal(t, "work-ledger", pinned.Notes[0].ID)
	assert.Equal(t, "Work Ledger", pinned.Notes[0].Title, "JSON keeps the title; only the text listing drops it")
	assert.Equal(t, tokens, pinned.Notes[0].Tokens)
	assert.Equal(t, tokens, pinned.Notes[0].SessionTokens)
	assert.Equal(t, "v1", decodeSchema(t, jout.Bytes()))
}

func TestWritePinnedSection_NilPrintsNothing(t *testing.T) {
	require.NoError(t, writePinnedSection(io.Discard, nil))
}

func TestWritePinnedSection_WriteErrors(t *testing.T) {
	p := &query.DoctorPinned{
		Count: 1, Tokens: 10, Budget: memory.SessionPinnedBudget,
		Notes: []query.DoctorPinnedNote{{ID: "work-ledger", Tokens: 10}},
	}
	require.Error(t, writePinnedSection(&failAfterNWriter{ok: 0}, p), "the count line")
	require.Error(t, writePinnedSection(&failAfterNWriter{ok: 1}, p), "the note line")
	p.OverBudget = true
	require.Error(t, writePinnedSection(&failAfterNWriter{ok: 2}, p), "the over-budget warning")
}

func TestDoctor_LongPinnedNoteFitsAtSessionStart(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_NO_UPDATE_CHECK", "1")
	t.Setenv("VAULTMIND_NO_SERVE", "1")
	// The hook may override its limits; doctor measures the shared defaults.
	t.Setenv("VAULTMIND_PINNED_BUDGET", "1")
	t.Setenv("VAULTMIND_PINNED_EXCERPT", "1")
	body := strings.Repeat("word ", 5600)
	dir := buildPinnedVault(t, map[string]string{
		"ledger.md": pinnedNote("work-ledger", "Work Ledger", true, body),
	})
	tokens := memory.EstimateTokens(body)
	require.Equal(t, 7000, tokens)

	jout, _, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err)
	pinned := decodeDoctorPinned(t, jout.Bytes())
	assert.Equal(t, tokens, pinned.Tokens)
	assert.InDelta(t, memory.SessionPinnedExcerpt, pinned.SessionTokens, 2)
	assert.Equal(t, memory.SessionPinnedBudget, pinned.Budget)
	assert.False(t, pinned.OverBudget)
	assert.Equal(t, []string{}, pinned.Omitted)
	require.Len(t, pinned.Notes, 1)
	assert.Equal(t, tokens, pinned.Notes[0].Tokens)
	assert.Equal(t, pinned.SessionTokens, pinned.Notes[0].SessionTokens)

	out, _, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), fmt.Sprintf("Pinned notes: 1 (%d tokens at session start, %d in full; default budget %d)", pinned.SessionTokens, tokens, memory.SessionPinnedBudget))
	assert.NotContains(t, out.String(), "unpin")
	assert.NotContains(t, out.String(), "pinned notes that would not load")

	// Compare with the command the hook invokes, using its default settings.
	loaded, _, err := runRootCmd(t, "note", "pinned", "--vault", dir, "--budget", itoa(memory.SessionPinnedBudget), "--excerpt", itoa(memory.SessionPinnedExcerpt), "--json")
	require.NoError(t, err)
	var hook struct {
		Result struct {
			Tokens int `json:"tokens"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(loaded.Bytes(), &hook))
	assert.Equal(t, hook.Result.Tokens, pinned.SessionTokens)
}

func TestDoctor_WarnsWhenPinnedNotesWouldNotLoadAtSessionStart(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_NO_UPDATE_CHECK", "1")
	t.Setenv("VAULTMIND_NO_SERVE", "1")
	body := strings.Repeat("word ", 500)
	notes := make(map[string]string)
	for _, id := range []string{"note-a", "note-b", "note-c", "note-d", "note-e"} {
		notes[id+".md"] = pinnedNote(id, id, true, body)
	}
	dir := buildPinnedVault(t, notes)

	jout, _, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err)
	pinned := decodeDoctorPinned(t, jout.Bytes())
	assert.Equal(t, 5, pinned.Count)
	assert.Equal(t, 5*memory.EstimateTokens(body), pinned.Tokens)
	assert.True(t, pinned.OverBudget)
	assert.Equal(t, []string{"note-d", "note-e"}, pinned.Omitted)
	require.Len(t, pinned.Notes, 5, "full costs include notes omitted at session start")
	for i, note := range pinned.Notes {
		assert.Equal(t, memory.EstimateTokens(body), note.Tokens)
		if i < 3 {
			assert.InDelta(t, memory.SessionPinnedExcerpt, note.SessionTokens, 2)
		} else {
			assert.Zero(t, note.SessionTokens, "an omitted note delivers no tokens")
		}
	}
	assert.Equal(t, 3*pinned.Notes[0].SessionTokens, pinned.SessionTokens)

	out, _, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "pinned notes that would not load at session start: note-d, note-e — unpin or shorten")
	assert.Contains(t, text, "Issues:")
	assert.NotContains(t, text, "Issues: 0 errors, 0 warnings", "the pinned warning must be counted")
}

func TestDoctor_NoPinnedNotesKeepsOutput(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_NO_UPDATE_CHECK", "1")
	t.Setenv("VAULTMIND_NO_SERVE", "1")
	dir := buildPinnedVault(t, map[string]string{
		"ledger.md": pinnedNote("work-ledger", "Work Ledger", false, "short ledger"),
	})
	out, _, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), fmt.Sprintf("Pinned notes: 0 (0 tokens; session-start budget %d)\n", memory.SessionPinnedBudget))
	assert.NotContains(t, out.String(), "unpin")

	jout, _, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err)
	pinned := decodeDoctorPinned(t, jout.Bytes())
	assert.Zero(t, pinned.Count)
	assert.Zero(t, pinned.Tokens)
	assert.Zero(t, pinned.SessionTokens)
	assert.False(t, pinned.OverBudget)
	assert.Equal(t, []string{}, pinned.Omitted)
	assert.NotNil(t, pinned.Notes)
	assert.Empty(t, pinned.Notes)
}

type doctorPinnedJSON struct {
	Count         int      `json:"count"`
	Tokens        int      `json:"tokens"`
	SessionTokens int      `json:"session_tokens"`
	Omitted       []string `json:"omitted"`
	Budget        int      `json:"budget"`
	OverBudget    bool     `json:"over_budget"`
	Notes         []struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		Tokens        int    `json:"tokens"`
		SessionTokens int    `json:"session_tokens"`
	} `json:"notes"`
}

func decodeDoctorPinned(t *testing.T, raw []byte) doctorPinnedJSON {
	t.Helper()
	var env struct {
		Result struct {
			Pinned doctorPinnedJSON `json:"pinned"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.Result.Pinned
}

func decodeSchema(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		SchemaVersion string `json:"schema_version"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.SchemaVersion
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
