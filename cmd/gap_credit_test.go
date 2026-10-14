package cmd

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsk_CreditsTheNoteThatClosedAGap(t *testing.T) {
	dir := gapCommandVault(t)
	const question = "How does quartz work?"
	noteID := closeGapWithFinding(t, dir, question, "How does quartz work? Quartz rebuilds the site when a note is saved.")

	text, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--budget", "4000")
	require.NoError(t, err, errOut.String())
	line := regexp.MustCompile(`  ↳ answers a question first investigated \d{4}-\d{2}-\d{2}: "How does quartz work\?"`)
	assert.Regexp(t, line, text.String())
	assert.Equal(t, 1, countCreditLines(text.String()))

	pointers, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--pointers-only")
	require.NoError(t, err, errOut.String())
	assert.NotContains(t, pointers.String(), "↳ answers a question")

	raw, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	credit := decodeAskCredit(t, raw.Bytes())
	assert.Equal(t, "v1", credit.SchemaVersion)
	assert.Equal(t, question, credit.Question)
	assert.Equal(t, noteID, credit.NoteID)
	assert.NotEmpty(t, credit.OpenedAt)
	assert.NotEmpty(t, credit.ClosedAt)

	t.Setenv(experiment.EnvCaller, "vaultmind-userprompt-hook")
	hooked, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--budget", "4000")
	require.NoError(t, err, errOut.String())
	assert.Regexp(t, line, hooked.String(), "a hook caller still sees the credit line")
	t.Setenv(experiment.EnvCaller, "")

	t.Setenv(experiment.EnvUserSessionID, "credit-session")
	first, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--budget", "4000", "--dedup-window", "10m")
	require.NoError(t, err, errOut.String())
	assert.Regexp(t, line, first.String())
	second, errOut, err := runRootCmd(t, "ask", question, "--vault", dir, "--budget", "4000", "--dedup-window", "10m")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, second.String(), "shown earlier this session")
	assert.NotContains(t, second.String(), "↳ answers a question", "dedup does not repeat the credit line")
}

func TestDoctor_GapLoopReadsTheClaim(t *testing.T) {
	dir := gapCommandVault(t)
	t.Setenv(experiment.EnvCaller, "")
	const question = "How does quartz work?"
	noteID := closeGapWithFinding(t, dir, question, "Quartz rebuilds the site when a note is saved.")
	seedGapClosingCite(t, dir, noteID)

	human, errOut, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, human.String(), "gap-closing notes: 1 surfaced, 1 cited")

	raw, errOut, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var doc struct {
		SchemaVersion string `json:"schema_version"`
		Result        struct {
			GapLoop struct {
				Opened          int      `json:"opened"`
				Closed          int      `json:"closed"`
				Median          *float64 `json:"median_days_to_close"`
				GapClosingCited int      `json:"gap_closing_cited"`
			} `json:"gap_loop"`
			MemoryUse struct {
				GapClosingSurfaced int `json:"gap_closing_surfaced"`
				GapClosingCited    int `json:"gap_closing_cited"`
			} `json:"memory_use"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &doc))
	assert.Equal(t, "v1", doc.SchemaVersion)
	assert.Equal(t, 1, doc.Result.GapLoop.Opened)
	assert.Equal(t, 1, doc.Result.GapLoop.Closed)
	require.NotNil(t, doc.Result.GapLoop.Median)
	assert.Equal(t, 1, doc.Result.GapLoop.GapClosingCited)
	assert.Equal(t, 1, doc.Result.MemoryUse.GapClosingSurfaced)
	assert.Equal(t, 1, doc.Result.MemoryUse.GapClosingCited)

	var loop map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(mustGapLoop(t, raw.Bytes()), &loop))
	for _, key := range []string{"opened", "closed", "median_days_to_close", "gap_closing_cited"} {
		_, ok := loop[key]
		assert.True(t, ok, "gap_loop.%s", key)
	}
}

func closeGapWithFinding(t *testing.T, dir, question, finding string) string {
	t.Helper()
	opened, errOut, err := runRootCmd(t, "note", "gap", question, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var gapEnv NoteGapEnvelope
	require.NoError(t, json.Unmarshal(opened.Bytes(), &gapEnv))
	proposed, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir, "--gap", gapEnv.Result.ID, "--finding", finding,
		"--close-band", "not-a-real-band", "--json")
	require.NoError(t, err, errOut.String())
	var draft NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(proposed.Bytes(), &draft))
	_, errOut, err = runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	return draft.Result.ID
}

func seedGapClosingCite(t *testing.T, vault, noteID string) {
	t.Helper()
	db, err := openExperimentDB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	sid, err := db.StartSessionWithCaller(vault, "vaultmind-userprompt-hook", map[string]any{experiment.MetaUserSessionID: "credit-session"})
	require.NoError(t, err)
	at := time.Now().UTC().Add(-time.Minute)
	askID, err := db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventAsk, VaultPath: vault,
		Data: experiment.BuildAskEventData(experiment.AskEventParams{
			RetrievalMode: "keyword", BodyDelivered: true,
			TopHits: []experiment.RetrievalHit{{NoteID: noteID}, {NoteID: "plain-note"}},
		}),
	})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE events SET timestamp = ? WHERE event_id = ?`, at.Format(time.RFC3339Nano), askID)
	require.NoError(t, err)
	for _, id := range []string{noteID, "plain-note"} {
		ev, evErr := db.LogEvent(experiment.Event{
			SessionID: sid, Type: experiment.EventUseEvidence, VaultPath: vault,
			Data: map[string]any{"note_id": id, "evidence": "cited", "surfaced_event_id": askID},
		})
		require.NoError(t, evErr)
		_, err = db.Exec(`UPDATE events SET timestamp = ? WHERE event_id = ?`, at.Add(time.Second).Format(time.RFC3339Nano), ev)
		require.NoError(t, err)
	}
}

type askCredit struct {
	SchemaVersion string
	NoteID        string
	Question      string
	OpenedAt      string
	ClosedAt      string
}

func decodeAskCredit(t *testing.T, raw []byte) askCredit {
	t.Helper()
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Result        struct {
			TopHits []struct {
				ID         string          `json:"id"`
				AnswersGap json.RawMessage `json:"answers_gap"`
			} `json:"top_hits"`
			Context struct {
				Target *struct {
					ID         string          `json:"id"`
					AnswersGap json.RawMessage `json:"answers_gap"`
				} `json:"target"`
				Context []struct {
					ID         string          `json:"id"`
					AnswersGap json.RawMessage `json:"answers_gap"`
				} `json:"context"`
			} `json:"context"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	out := askCredit{SchemaVersion: env.SchemaVersion}
	take := func(id string, raw json.RawMessage) {
		if len(raw) == 0 || string(raw) == "null" {
			return
		}
		var gap struct {
			Question string `json:"question"`
			OpenedAt string `json:"opened_at"`
			ClosedAt string `json:"closed_at"`
		}
		require.NoError(t, json.Unmarshal(raw, &gap))
		if out.NoteID == "" {
			out.NoteID = id
			out.Question = gap.Question
			out.OpenedAt = gap.OpenedAt
			out.ClosedAt = gap.ClosedAt
		}
	}
	for _, hit := range env.Result.TopHits {
		take(hit.ID, hit.AnswersGap)
	}
	if env.Result.Context.Target != nil {
		take(env.Result.Context.Target.ID, env.Result.Context.Target.AnswersGap)
	}
	for _, item := range env.Result.Context.Context {
		take(item.ID, item.AnswersGap)
	}
	require.NotEmpty(t, out.NoteID, "json answer did not carry answers_gap: %s", raw)
	return out
}

func countCreditLines(text string) int {
	return len(regexp.MustCompile(`(?m)^  ↳ answers a question`).FindAllString(text, -1))
}

func mustGapLoop(t *testing.T, raw []byte) []byte {
	t.Helper()
	var env struct {
		Result struct {
			GapLoop json.RawMessage `json:"gap_loop"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	require.NotEmpty(t, env.Result.GapLoop)
	return env.Result.GapLoop
}
