package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Claude Code replaces a SessionStart hook's output with a ~2 KB preview once it
// passes about 10,000 characters (#355): the agent then starts from a preview of
// its identity while the hook logs success. The persona hook keeps its output
// under a cap, and when it cannot deliver at all it says so on stdout (#356).

const deliveryCap = 9000

// sizedStub prints sections whose sizes match a real 30-arc identity vault, with
// `arc recite` growing with --budget the way the real command does (~5 chars per
// budget token) and naming what it left out.
func sizedStub(identityChars int) string {
	return "#!/bin/bash\n" +
		"budget=3000; prev=\n" +
		"for a in \"$@\"; do [ \"$prev\" = --budget ] && budget=$a; prev=$a; done\n" +
		"fill() { head -c \"$1\" /dev/zero | tr '\\0' \"$2\" | fold -w 100; echo; }\n" +
		"case \"$1\" in\n" +
		"  arc) fill $((budget * 5)) a; echo 'ARCS-NAMED-REST' ;;\n" +
		"  ask) if [ \"$2\" = 'who am I' ]; then fill " + itoa(identityChars) + " i; else fill 1100 c; fi ;;\n" +
		"  self) fill 1300 s ;;\n" +
		"esac\n" +
		"exit 0\n"
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func lastSidecar(t *testing.T, env []string) map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(envValue(env, "HOME"), ".vaultmind", "persona-eval", "*-injection.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "the hook must write a sidecar record")
	raw, err := os.ReadFile(files[len(files)-1]) //nolint:gosec // test fixture path
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(raw, &rec), "sidecar must stay valid JSON: %s", raw)
	return rec
}

func TestPersonaHook_ServedOutputStaysUnderTheDeliveryCap(t *testing.T) {
	project, env := installPersonaHook(t, sizedStub(1200))
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runPersonaHookSession(t, project, env, "s-cap")

	assert.LessOrEqual(t, len(out), deliveryCap, "served output must fit under the cap (bytes)")
	assert.Contains(t, out, "IDENTITY CONTEXT:")
	assert.Contains(t, out, "aaaa", "the arc layer gets the room the other sections leave")
	assert.Contains(t, out, "CURRENT CONTEXT:")
	rec := lastSidecar(t, env)
	assert.Equal(t, true, rec["injection_success"])
	assert.EqualValues(t, len(out), rec["output_bytes"], "the log records what was actually printed")
}

func TestPersonaHook_HugeIdentityStillFitsAndPointsToTheRest(t *testing.T) {
	project, env := installPersonaHook(t, sizedStub(12000))
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runPersonaHookSession(t, project, env, "s-huge")

	assert.LessOrEqual(t, len(out), deliveryCap)
	assert.Contains(t, out, "vaultmind arc recite", "what was left out is named with the way to read it")
	rec := lastSidecar(t, env)
	assert.NotEmpty(t, rec["capped"], "the log says what was dropped")
}

func TestPersonaHook_FailedLoadSaysSoOnStdout(t *testing.T) {
	stub := "#!/bin/bash\n" +
		"if [ \"$1\" = ask ]; then echo 'Error: opening index: database is locked (5)' >&2; exit 1; fi\n" +
		"exit 0\n"
	project, env := installPersonaHook(t, stub)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runPersonaHookSession(t, project, env, "s-locked")

	assert.Contains(t, out, "IDENTITY NOT LOADED", "a failed delivery must reach the agent, not only stderr")
	assert.Contains(t, out, "database is locked", "with the reason")
	assert.Contains(t, out, "vaultmind arc recite", "and the way to recover")
	assert.LessOrEqual(t, len(out), deliveryCap)
	rec := lastSidecar(t, env)
	assert.Equal(t, false, rec["injection_success"])
	assert.True(t, strings.Contains(rec["error"].(string), "database is locked"), "the log keeps the error: %v", rec["error"])
}
