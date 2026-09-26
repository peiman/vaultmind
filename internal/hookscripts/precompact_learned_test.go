package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At compaction the hook asks for what the segment taught about the code, not
// only for a desk entry about the agent: search the knowledge vault for each
// lesson, write only what is missing — with paths: naming the code — and fix
// what is stale. A knowledge vault is one without arcs/ (the identity vault
// has them).

const learnedStep = "LEARNED ABOUT THE CODE"

type precompactEnv struct {
	identity, kb string
	env          []string
}

func newPrecompactEnv(t *testing.T, withKB bool) precompactEnv {
	t.Helper()
	project := t.TempDir()
	identity := filepath.Join(project, "vaultmind-identity")
	require.NoError(t, os.MkdirAll(filepath.Join(identity, "arcs"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(identity, "journal"), 0o750))
	vaults := identity
	kb := ""
	if withKB {
		kb = filepath.Join(project, "kb")
		require.NoError(t, os.MkdirAll(filepath.Join(kb, "concepts"), 0o750))
		vaults += "," + kb
	}
	return precompactEnv{identity: identity, kb: kb, env: []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + t.TempDir(),
		"CLAUDE_PROJECT_DIR=" + project,
		"VAULTMIND_VAULT=" + identity,
		"VAULTMIND_VAULTS=" + vaults,
	}}
}

func precompactMessage(t *testing.T, env []string) string {
	t.Helper()
	out, _ := runHookScript(t, "precompact-preserve.sh", env, `{"session_id":"s-learned","trigger":"auto"}`)
	var got struct {
		SystemMessage string `json:"systemMessage"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	return got.SystemMessage
}

func TestPrecompact_AsksWhatTheSegmentTaughtAboutTheCode(t *testing.T) {
	e := newPrecompactEnv(t, true)

	msg := precompactMessage(t, e.env)
	assert.Contains(t, msg, learnedStep)
	assert.Contains(t, msg, "vaultmind ask", "search before writing")
	assert.Contains(t, msg, e.kb, "names the knowledge vault to write into")
	assert.Contains(t, msg, "paths:", "a new note names the code it is about")
}

func TestPrecompact_WithoutAKnowledgeVaultThereIsNoCodeStep(t *testing.T) {
	e := newPrecompactEnv(t, false)

	assert.NotContains(t, precompactMessage(t, e.env), learnedStep)
}

func TestPrecompact_TheCodeStepStaysAfterTheDeskEntryIsWritten(t *testing.T) {
	e := newPrecompactEnv(t, true)
	today := time.Now().Format("2006-01-02")
	require.NoError(t, os.WriteFile(filepath.Join(e.identity, "journal", today+"-done.md"), []byte("x"), 0o600))

	msg := precompactMessage(t, e.env)
	assert.Contains(t, msg, "already exists", "the desk entry is not asked for again")
	assert.Contains(t, msg, learnedStep, "but what was learned about the code still is")
}
