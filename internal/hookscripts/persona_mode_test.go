package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The explore arm of the persona hook: the identity is not handed over, the
// agent is pointed at its vault and asked to look. VAULTMIND_PERSONA_MODE picks
// the arm — served (default), explore, or alternate (split by session id, so a
// session keeps its arm across compactions and the two can be compared).

const exploreMarker = "IDENTITY — EXPLORE"

// personaCallLogStub records every vaultmind call so a test can assert the
// explore arm makes none.
const personaCallLogStub = "#!/bin/bash\n" +
	"echo \"$*\" >> \"$HOME/calls.log\"\n" +
	"if [ \"$1\" = \"arc\" ]; then echo 'SENTINEL-ARC-BODY'; exit 0; fi\n" +
	"if [ \"$1\" = ask ]; then echo '  0.42  identity-who-i-am   Who I Am'; fi\n" +
	"exit 0\n"

func runPersonaHookSession(t *testing.T, project string, env []string, sessionID string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"session_id": sessionID, "hook_event_name": "SessionStart", "source": "startup",
	})
	require.NoError(t, err)
	cmd := exec.Command("/bin/bash", filepath.Join(project, ".claude", "scripts", "load-persona.sh"))
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(payload))
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoError(t, cmd.Run(), "hook must exit 0 (stderr: %s)", errb.String())
	return out.String()
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

// sidecarModes returns the persona_mode of every injection record written.
func sidecarModes(t *testing.T, env []string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(envValue(env, "HOME"), ".vaultmind", "persona-eval", "*-injection.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "the hook must write a sidecar record")
	var modes []string
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // test fixture path
		require.NoError(t, err)
		var rec map[string]any
		require.NoError(t, json.Unmarshal(raw, &rec), "sidecar must stay valid JSON: %s", raw)
		mode, _ := rec["persona_mode"].(string)
		modes = append(modes, mode)
	}
	return modes
}

func TestPersonaMode_ExplorePointsAtTheVaultAndServesNothing(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	env = append(env, "VAULTMIND_PERSONA_MODE=explore")

	out := runPersonaHookSession(t, project, env, "s-explore")

	assert.Contains(t, out, exploreMarker)
	assert.Contains(t, out, filepath.Join(project, "vaultmind-identity"), "the agent must be told where to look")
	assert.Contains(t, out, "vaultmind arc recite --vault", "the ways in must be named exactly")
	assert.NotContains(t, out, "SENTINEL-ARC-BODY", "explore must not hand over the arcs")
	assert.NotContains(t, out, "IDENTITY CONTEXT:", "explore must not serve the identity block")
	_, err := os.Stat(filepath.Join(envValue(env, "HOME"), "calls.log"))
	assert.True(t, os.IsNotExist(err), "explore must make no vaultmind calls on the agent's behalf")
	assert.Equal(t, []string{"explore"}, sidecarModes(t, env), "the arm must be recorded for measurement")
}

func TestPersonaMode_DefaultIsServedAndRecorded(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)

	out := runPersonaHookSession(t, project, env, "s-served")

	assert.Contains(t, out, "IDENTITY CONTEXT:")
	assert.Contains(t, out, "SENTINEL-ARC-BODY")
	assert.NotContains(t, out, exploreMarker)
	assert.Equal(t, []string{"served"}, sidecarModes(t, env))
}

func TestPersonaMode_ExploreListsEveryConfiguredVault(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	desk := filepath.Join(project, "desk")
	require.NoError(t, os.MkdirAll(desk, 0o750))
	env = append(env, "VAULTMIND_PERSONA_MODE=explore",
		"VAULTMIND_VAULTS="+filepath.Join(project, "vaultmind-identity")+","+desk)

	out := runPersonaHookSession(t, project, env, "s-vaults")

	assert.Contains(t, out, desk, "every vault in VAULTMIND_VAULTS is a place to look")
	assert.Equal(t, 1, strings.Count(out, filepath.Join(project, "vaultmind-identity")+"\n"),
		"the identity vault is listed once even when it is also in VAULTMIND_VAULTS")
}

// Alternate splits sessions between the arms by session id: both arms occur,
// and the same session always gets the same arm (a compaction must not switch
// a session to the other arm mid-way).
func TestPersonaMode_AlternateSplitsBySessionAndIsStable(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	env = append(env, "VAULTMIND_PERSONA_MODE=alternate")

	arms := map[bool]bool{}
	for _, id := range []string{"a1", "b2", "c3", "d4", "e5", "f6", "g7", "h8"} {
		first := strings.Contains(runPersonaHookSession(t, project, env, id), exploreMarker)
		again := strings.Contains(runPersonaHookSession(t, project, env, id), exploreMarker)
		assert.Equal(t, first, again, "session %s must keep its arm", id)
		arms[first] = true
	}
	assert.Len(t, arms, 2, "alternate must put sessions in both arms")
}

func TestPersonaMode_UnknownModeFallsBackToServed(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	env = append(env, "VAULTMIND_PERSONA_MODE=exploer")

	out := runPersonaHookSession(t, project, env, "s-typo")

	assert.Contains(t, out, "IDENTITY CONTEXT:", "a typo must never leave the agent with no identity")
	assert.Equal(t, []string{"served"}, sidecarModes(t, env))
}

// The served record carries the right values in the right fields: a shifted
// printf argument still parses as JSON, so checking only the arm misses it.
func TestPersonaMode_ServedRecordHasItsFieldsInPlace(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)

	runPersonaHookSession(t, project, env, "s-fields")

	files, err := filepath.Glob(filepath.Join(envValue(env, "HOME"), ".vaultmind", "persona-eval", "*-injection.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(raw, &rec), "sidecar must be valid JSON: %s", raw)
	assert.Equal(t, filepath.Join(project, "vaultmind-identity"), rec["vault_path"])
	assert.Equal(t, "s-fields", rec["session_id"])
	assert.Greater(t, rec["identity_length"], 0.0, "the served identity length must land in its own field")
	assert.Equal(t, true, rec["injection_success"])
}

// A session assigned to explore stays in explore in the log even when the
// vault is missing, or the comparison counts it in the wrong arm.
func TestPersonaMode_MissingVaultKeepsTheAssignedArmInTheLog(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	require.NoError(t, os.RemoveAll(filepath.Join(project, "vaultmind-identity")))
	env = append(env, "VAULTMIND_PERSONA_MODE=explore")

	runPersonaHookSession(t, project, env, "s-no-vault")

	assert.Equal(t, []string{"explore"}, sidecarModes(t, env))
}

// Two sessions starting in the same second each keep their record. The file
// was named by the second alone, so the later one overwrote the earlier, live
// on 2026-09-25. The clock is pinned by a date stub to make the collision
// certain rather than likely.
func TestPersonaMode_SameSecondSessionsKeepSeparateRecords(t *testing.T) {
	project, env := installPersonaHook(t, personaCallLogStub)
	binDir := strings.SplitN(envValue(env, "PATH"), ":", 2)[0]
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "date"), []byte("#!/bin/sh\necho 20260925T120000\n"), 0o700)) //nolint:gosec // test fixture
	env = append(env, "VAULTMIND_PERSONA_MODE=explore")

	runPersonaHookSession(t, project, env, "s-one")
	runPersonaHookSession(t, project, env, "s-two")

	assert.Len(t, sidecarModes(t, env), 2, "each session must keep its own record")
}
