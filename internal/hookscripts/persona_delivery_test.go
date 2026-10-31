package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"

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

// #357: the cap is per hook, so the arc layer gets a hook of its own. This stub
// models the measured 30-arc vault: ~350 bytes an arc at --excerpt 120 (10.5k,
// over the cap) and ~287 at 70 (8.6k, all thirty fit).
const arcLayerStub = "#!/bin/bash\n" +
	"budget=3000; excerpt=120; prev=\n" +
	"for a in \"$@\"; do [ \"$prev\" = --budget ] && budget=$a; [ \"$prev\" = --excerpt ] && excerpt=$a; prev=$a; done\n" +
	"fill() { head -c \"$1\" /dev/zero | tr '\\0' \"$2\" | fold -w 100; echo; }\n" +
	"case \"$1\" in\n" +
	"  arc) total=$(( (200 + excerpt * 5 / 4) * 30 ))\n" +
	"       if [ $((budget * 5)) -lt \"$total\" ]; then fill $((budget * 5)) a; echo 'SOME arc(s) DID NOT FIT'; else fill \"$total\" a; echo 'ALL-30-ARCS'; fi ;;\n" +
	"  ask) if [ \"$2\" = 'who am I' ]; then fill 3200 i; else fill 1100 c; fi ;;\n" +
	"  self) fill 1400 s ;;\n" +
	"esac\n" +
	"exit 0\n"

// installArcsHook adds the embedded load-arcs.sh beside load-persona.sh and
// wires it in the project's settings, the way `hooks install` does both.
func installArcsHook(t *testing.T, project string) {
	t.Helper()
	copyArcsScript(t, project)
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "settings.json"),
		[]byte(`{"hooks":{"SessionStart":[{"matcher":"startup|clear|compact","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-arcs.sh"}]}]}}`), 0o600))
}

func copyArcsScript(t *testing.T, project string) {
	t.Helper()
	body, ok := hookscripts.All()["load-arcs.sh"]
	require.True(t, ok, "load-arcs.sh must be embedded")
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "scripts", "load-arcs.sh"), body, 0o600))
}

// A load-arcs.sh nobody wired must not make the arcs vanish: the persona hook
// then delivers them itself (the worst case is twice, never not at all).
func TestPersonaHook_KeepsTheArcsWhenTheArcsHookIsNotWired(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	copyArcsScript(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runSessionStartScript(t, project, env, "load-persona.sh", "s-unwired")

	assert.Contains(t, out, "aaaa", "an unwired arcs hook delivers nothing, so the persona hook must")
	assert.LessOrEqual(t, len(out), deliveryCap)
}

func runSessionStartScript(t *testing.T, project string, env []string, script, sessionID string) string {
	t.Helper()
	return runSessionStartScriptFrom(t, project, env, script, sessionID, "startup")
}

func runSessionStartScriptFrom(t *testing.T, project string, env []string, script, sessionID, source string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"session_id": sessionID, "hook_event_name": "SessionStart", "source": source})
	require.NoError(t, err)
	cmd := exec.Command("/bin/bash", filepath.Join(project, ".claude", "scripts", script))
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(payload))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), "%s must exit 0 (stderr: %s)", script, errb.String())
	return out.String()
}

func TestArcsHook_DeliversTheWholeArcLayerUnderTheCap(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runSessionStartScript(t, project, env, "load-arcs.sh", "s-arcs")

	assert.LessOrEqual(t, len(out), deliveryCap, "the arcs hook has the same cap (bytes)")
	assert.Contains(t, out, "ALL-30-ARCS", "a shorter excerpt that fits every arc beats fewer arcs at full length")
	assert.NotContains(t, out, "IDENTITY CONTEXT:", "the arcs hook delivers only the arc layer")
}

func TestPersonaHook_LeavesTheArcsToTheArcsHookWhenItIsInstalled(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runSessionStartScript(t, project, env, "load-persona.sh", "s-split")

	assert.LessOrEqual(t, len(out), deliveryCap)
	assert.Contains(t, out, "IDENTITY CONTEXT:")
	assert.NotContains(t, out, "aaaa", "with load-arcs.sh beside it, the persona hook leaves the arcs to that hook")
	assert.Contains(t, out, "MEMORY STATE — IDENTITY VAULT:", "the room the arcs freed goes to the memory state")
}

// Claude Code reads its hooks once, when the process starts. A session started
// before load-arcs.sh was wired never runs it, yet the settings file now names
// it. Trusting the file there lost the arcs at every compact (seen 2026-10-31
// in two keepers). After startup the persona hook leaves the arcs to that hook
// only once it has seen the hook run in this same harness process.
func TestPersonaHook_CompactInAHarnessThatNeverRanTheArcsHookKeepsTheArcs(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	for _, source := range []string{"compact", "clear", "resume"} {
		out := runSessionStartScriptFrom(t, project, env, "load-persona.sh", "s-old-harness", source)
		assert.Contains(t, out, "aaaa", "%s: the arcs hook never ran in this harness, so the persona hook must deliver the arcs", source)
		assert.LessOrEqual(t, len(out), deliveryCap)
	}
}

func TestPersonaHook_CompactAfterTheArcsHookRanLeavesTheArcsToIt(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	runSessionStartScriptFrom(t, project, env, "load-arcs.sh", "s-live", "startup")
	out := runSessionStartScriptFrom(t, project, env, "load-persona.sh", "s-live", "compact")

	assert.NotContains(t, out, "aaaa", "the arcs hook ran in this harness, so it delivers the arcs at compact too")
	assert.Contains(t, out, "IDENTITY CONTEXT:")
}

// An agent that lost its arcs may run load-arcs.sh by hand to get them back.
// That run must not count as the harness running the hook, or every later
// compact in the session drops the arcs again.
func TestPersonaHook_AHandRunOfTheArcsHookDoesNotCountAsTheHarness(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	cmd := exec.Command("/bin/bash", filepath.Join(project, ".claude", "scripts", "load-arcs.sh"))
	cmd.Env = env
	cmd.Stdin = strings.NewReader("")
	require.NoError(t, cmd.Run(), "a hand run with no payload must exit 0")
	out := runSessionStartScriptFrom(t, project, env, "load-persona.sh", "s-hand", "compact")

	assert.Contains(t, out, "aaaa", "a hand run is not the harness running the arcs hook")
}

func TestArcsHook_ExploreArmDeliversNothing(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=explore")

	out := runSessionStartScript(t, project, env, "load-arcs.sh", "s-explore-arcs")

	assert.Empty(t, out, "the explore arm hands over no identity, arcs included")
}

func TestArcsHook_KeepsItsOwnSidecarRecord(t *testing.T) {
	project, env := installPersonaHook(t, arcLayerStub)
	installArcsHook(t, project)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	runSessionStartScript(t, project, env, "load-persona.sh", "s-both")
	runSessionStartScript(t, project, env, "load-arcs.sh", "s-both")

	files, err := filepath.Glob(filepath.Join(envValue(env, "HOME"), ".vaultmind", "persona-eval", "*-injection.json"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "both hooks fire in the same second; neither record may overwrite the other")
	var parts []string
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // test fixture path
		require.NoError(t, err)
		var rec map[string]any
		require.NoError(t, json.Unmarshal(raw, &rec), "sidecar must stay valid JSON: %s", raw)
		part, _ := rec["persona_part"].(string)
		parts = append(parts, part)
		if part == "arcs" {
			assert.EqualValues(t, 70, rec["arc_excerpt"], "the record says which excerpt fitted")
		}
	}
	assert.ElementsMatch(t, []string{"identity", "arcs"}, parts)
}
