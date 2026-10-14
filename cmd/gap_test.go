package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gapVaultConfig = "types:\n  concept:\n    required: [title]\n    optional: [aliases, tags, sources]\n    template: .vaultmind/templates/concept.md\n"

func gapCommandVault(t *testing.T) string {
	t.Helper()
	isolateXDG(t)
	return setupNoteCreateVault(t, gapVaultConfig, map[string]string{
		".vaultmind/templates/concept.md": "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\n---\n",
	})
}

func TestContract_NoteGapJSON(t *testing.T) {
	dir := gapCommandVault(t)
	t.Setenv(experiment.EnvUserSessionID, "sess-gap")
	const question = "How does quartz work?"

	out, errOut, err := runRootCmd(t, "note", "gap", question, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var env NoteGapEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note gap", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "created", env.Result.Mode)
	assert.Equal(t, "gap-how-does-quartz-work", env.Result.ID)
	assert.Equal(t, "gaps/how-does-quartz-work.md", env.Result.Path)
	assert.Equal(t, question, env.Result.Question)
	assert.Equal(t, "open", env.Result.Status)
	assert.NotNil(t, env.Result.AskedHits)
	assert.Equal(t, 1, env.Result.Opened)
	assert.Equal(t, 0, env.Result.Closed)
	assert.Nil(t, env.Result.MedianDaysToClose)
	assert.Equal(t, dir, env.Meta.VaultPath)
	assert.NotEmpty(t, env.Meta.IndexHash)

	raw, err := os.ReadFile(filepath.Join(dir, "gaps", "how-does-quartz-work.md"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "type: gap")
	assert.Contains(t, string(raw), "status: open")
	assert.Contains(t, string(raw), "session: sess-gap")
	assert.Contains(t, string(raw), "opened_at:")

	again, errOut, err := runRootCmd(t, "note", "gap", "  how   DOES quartz   work? ", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var existing NoteGapEnvelope
	require.NoError(t, json.Unmarshal(again.Bytes(), &existing))
	assert.Equal(t, "existing", existing.Result.Mode)
	assert.Equal(t, env.Result.ID, existing.Result.ID)

	human, errOut, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err, errOut.String())
	text := human.String()
	assert.Contains(t, text, "Open gaps: 1")
	assert.Contains(t, text, env.Result.ID)
	assert.Contains(t, text, question)
	assert.Contains(t, text, "0d")
	assert.Contains(t, text, "Gaps opened: 1")
	assert.Contains(t, text, "Gaps closed: 0")
	assert.Contains(t, text, "Median days to close: n/a")
	assert.Contains(t, text, "0 weak asks")

	doc, errOut, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var doctor struct {
		SchemaVersion string `json:"schema_version"`
		Result        struct {
			Gaps struct {
				Notes []struct {
					ID       string `json:"id"`
					Question string `json:"question"`
					AgeDays  int    `json:"age_days"`
				} `json:"notes"`
				Opened   int      `json:"opened"`
				Closed   int      `json:"closed"`
				Median   *float64 `json:"median_days_to_close"`
				Possible int      `json:"possible"`
			} `json:"gaps"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(doc.Bytes(), &doctor))
	assert.Equal(t, "v1", doctor.SchemaVersion)
	require.Len(t, doctor.Result.Gaps.Notes, 1)
	assert.Equal(t, env.Result.ID, doctor.Result.Gaps.Notes[0].ID)
	assert.Equal(t, question, doctor.Result.Gaps.Notes[0].Question)
	assert.Equal(t, 1, doctor.Result.Gaps.Opened)
	assert.Equal(t, 0, doctor.Result.Gaps.Closed)
	assert.Nil(t, doctor.Result.Gaps.Median)
	assert.Equal(t, 0, doctor.Result.Gaps.Possible)
}

func TestNoteGap_ProposeAcceptClosesIt(t *testing.T) {
	dir := gapCommandVault(t)
	const question = "How does quartz work?"
	opened, errOut, err := runRootCmd(t, "note", "gap", question, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var gapEnv NoteGapEnvelope
	require.NoError(t, json.Unmarshal(opened.Bytes(), &gapEnv))

	proposed, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--gap", gapEnv.Result.ID,
		"--finding", "Quartz rebuilds the site when a note is saved.",
		"--close-band", "not-a-real-band",
		"--json")
	require.NoError(t, err, "stdout would be empty\nstderr: %s", errOut.String())
	var draft NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(proposed.Bytes(), &draft))
	assert.Equal(t, "new", draft.Result.Mode)
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(draft.Result.Path)))
	require.NoError(t, err)
	assert.Contains(t, string(body), "closes_gap: "+gapEnv.Result.ID)

	_, errOut, err = runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	closed, err := os.ReadFile(filepath.Join(dir, "gaps", "how-does-quartz-work.md"))
	require.NoError(t, err)
	assert.Contains(t, string(closed), "status: closed")
	assert.Contains(t, string(closed), "closed_by: "+draft.Result.ID)
	assert.Contains(t, string(closed), "closed_at:")

	doc, errOut, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var doctor struct {
		Result struct {
			Gaps struct {
				Opened int      `json:"opened"`
				Closed int      `json:"closed"`
				Median *float64 `json:"median_days_to_close"`
				Notes  []struct {
					ID string `json:"id"`
				} `json:"notes"`
			} `json:"gaps"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(doc.Bytes(), &doctor))
	assert.Empty(t, doctor.Result.Gaps.Notes)
	assert.Equal(t, 1, doctor.Result.Gaps.Opened)
	assert.Equal(t, 1, doctor.Result.Gaps.Closed)
	require.NotNil(t, doctor.Result.Gaps.Median)
}

func possibleGapLogs(t *testing.T, state string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(state, "vaultmind", "possible-gaps", "*.jsonl"))
	require.NoError(t, err)
	return matches
}

func TestRecordPossibleGap_LogsWeakAsksOnly(t *testing.T) {
	isolateXDG(t)
	state := os.Getenv("XDG_STATE_HOME")
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	const queryText = "secret question"
	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceStrong})
	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceModerate})
	recordPossibleGap(askCmd, dir, queryText, nil)
	assert.Empty(t, possibleGapLogs(t, state))
	_, err := os.Stat(filepath.Join(dir, ".vaultmind", "possible-gaps.jsonl"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceWeak})
	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceNoMatch})
	logs := possibleGapLogs(t, state)
	require.Len(t, logs, 1)
	raw, err := os.ReadFile(logs[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), queryText)
	assert.Contains(t, string(raw), `"band":"weak"`)
	assert.Contains(t, string(raw), `"band":"no_match"`)

	viper.Set(config.KeyAppAskGapStoreQuery, true)
	t.Cleanup(func() { viper.Set(config.KeyAppAskGapStoreQuery, false) })
	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceWeak})
	raw, err = os.ReadFile(logs[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), queryText)

	humanDir := gapCommandVault(t)
	recordPossibleGap(askCmd, humanDir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceWeak})
	// The opt-in above would store the raw query. Turn it off for the doctor line.
	viper.Set(config.KeyAppAskGapStoreQuery, false)
	out, errOut, err := runRootCmd(t, "doctor", "--vault", humanDir)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), "1 weak asks since "+time.Now().UTC().Format("2006-01-02"))
}

func TestAsk_AppendsAPossibleGapForATie(t *testing.T) {
	dir := gapCommandVault(t)
	writeVaultNote(t, dir, "concepts/one.md", "concept-one", "One", "accepted", repeatQuery("tiequerytoken"))
	writeVaultNote(t, dir, "concepts/two.md", "concept-two", "Two", "accepted", repeatQuery("tiequerytoken"))
	rebuildVault(t, dir)

	out, errOut, err := runRootCmd(t, "ask", "tiequerytoken", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			TopHitConfidence string `json:"top_hit_confidence"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	logs := possibleGapLogs(t, os.Getenv("XDG_STATE_HOME"))
	_, vaultErr := os.Stat(filepath.Join(dir, ".vaultmind", "possible-gaps.jsonl"))
	assert.ErrorIs(t, vaultErr, os.ErrNotExist)
	if env.Result.TopHitConfidence == noisefloor.ConfidenceWeak || env.Result.TopHitConfidence == noisefloor.ConfidenceNoMatch {
		require.Len(t, logs, 1)
		raw, readErr := os.ReadFile(logs[0])
		require.NoError(t, readErr)
		assert.NotContains(t, string(raw), "tiequerytoken")
		assert.Contains(t, string(raw), env.Result.TopHitConfidence)
		return
	}
	assert.Empty(t, logs, "band %q must not be logged", env.Result.TopHitConfidence)
}

func TestRecordPossibleGap_SkipsHookCallers(t *testing.T) {
	isolateXDG(t)
	state := os.Getenv("XDG_STATE_HOME")
	dir := t.TempDir()
	const queryText = "secret question"
	for _, caller := range []string{"vaultmind-userprompt-hook", "auto-rag-guard"} {
		t.Run(caller, func(t *testing.T) {
			t.Setenv("VAULTMIND_CALLER", caller)
			recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceWeak})
			recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceNoMatch})
			assert.Empty(t, possibleGapLogs(t, state))
		})
	}
	t.Setenv("VAULTMIND_CALLER", "")
	recordPossibleGap(askCmd, dir, queryText, &query.AskResult{TopHitConfidence: noisefloor.ConfidenceWeak})
	logs := possibleGapLogs(t, state)
	require.Len(t, logs, 1)
	raw, err := os.ReadFile(logs[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"band":"weak"`)
	assert.NotContains(t, string(raw), queryText)
}
