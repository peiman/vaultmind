package hookscripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind-health.sh is the vault-agnostic SessionStart onboarding nudge
// (focalc field report P0). These tests exercise its state machine hermetically:
// a clean env, a PATH of system coreutils only (so the binary-missing state is
// reproducible regardless of what's installed on the test machine), and a
// stubbed `vaultmind` whose `doctor` output the hook must relay verbatim
// (tool = single source of truth for the index tier).

// healthHookPATH excludes the locations `vaultmind` installs to (~/go/bin,
// /usr/local/bin, /opt/homebrew/bin) while keeping the coreutils the script
// needs (cat, dirname, grep). So "binary missing" is deterministic here.
const healthHookPATH = "/usr/bin:/bin"

func runHealthHook(t *testing.T, projectDir, stubBin string, extraEnv ...string) (stdout, stderr string) {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs("vaultmind-health.sh")
	require.NoError(t, err)

	path := healthHookPATH
	if stubBin != "" {
		path = stubBin + ":" + path
	}
	cmd := exec.Command(bashPath, script)
	cmd.Env = append([]string{"PATH=" + path, "CLAUDE_PROJECT_DIR=" + projectDir}, extraEnv...)
	cmd.Stdin = strings.NewReader(`{"session_id":"test"}`)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	// Contract: the hook ALWAYS exits 0 — a broken/empty vault must never wedge
	// a session start.
	require.NoErrorf(t, cmd.Run(), "health hook must always exit 0 (stderr: %s)", errb.String())
	return out.String(), errb.String()
}

func projectWithVault(t *testing.T) string {
	t.Helper()
	return projectWithVaultNamed(t, "vaultmind-vault")
}

func projectWithVaultNamed(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
	return dir
}

// stubVaultmindFailing writes a fake `vaultmind` whose `doctor` writes errMsg to
// stderr and exits non-zero — the doctor-failure path the hook must surface
// honestly rather than launder into a success-shaped nudge.
func stubVaultmindFailing(t *testing.T, errMsg string, code int) string {
	t.Helper()
	bin := t.TempDir()
	body := fmt.Sprintf("#!/bin/bash\nif [ \"$1\" = doctor ]; then printf '%%s\\n' %s 1>&2; exit %d; fi\n",
		shellSingleQuote(errMsg), code)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "vaultmind"), []byte(body), 0o755))
	return bin
}

// stubVaultmind writes a fake `vaultmind` whose `doctor` prints doctorOut, in a
// fresh dir to prepend to PATH. The hook must read the tier from this, never
// re-derive it.
func stubVaultmind(t *testing.T, doctorOut string) string {
	t.Helper()
	bin := t.TempDir()
	body := "#!/bin/bash\nif [ \"$1\" = doctor ]; then printf '%s\\n' " + shellSingleQuote(doctorOut) + "; fi\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "vaultmind"), []byte(body), 0o755))
	return bin
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// No vault -> the hook stays completely silent (it only speaks when there's
// something to onboard).
func TestHealthHook_NoVaultIsSilent(t *testing.T) {
	out, errOut := runHealthHook(t, t.TempDir(), "")
	assert.Empty(t, strings.TrimSpace(out), "no vault must produce no stdout")
	assert.Empty(t, strings.TrimSpace(errOut))
}

// Vault present, binary missing -> name the install command (the P0 case: a
// vector-2 adopter who cloned a repo with a committed vault).
func TestHealthHook_VaultButNoBinaryNamesInstall(t *testing.T) {
	out, errOut := runHealthHook(t, projectWithVault(t), "")
	assert.Contains(t, out, "not installed")
	assert.Contains(t, out, "go install github.com/peiman/vaultmind@latest")
	assert.Empty(t, strings.TrimSpace(errOut),
		"the nudge must be on stdout, not stderr — SessionStart surfaces stdout to the agent")
}

// Binary present, index unbuilt -> name the build command.
func TestHealthHook_IndexUnbuiltNamesBuild(t *testing.T) {
	stub := stubVaultmind(t, "Embeddings: none (5 notes) — keyword-only retrieval")
	out, _ := runHealthHook(t, projectWithVault(t), stub)
	assert.Contains(t, out, "index not built yet")
	assert.Contains(t, out, "vaultmind index --embed")
}

// Index on MiniLM -> active-but-degraded, naming the upgrade path.
func TestHealthHook_MiniLMNamesUpgrade(t *testing.T) {
	stub := stubVaultmind(t, "Embeddings: dense 50/50 (minilm), sparse 0/50, colbert 0/50")
	out, _ := runHealthHook(t, projectWithVault(t), stub)
	assert.Contains(t, strings.ToLower(out), "degraded")
	assert.Contains(t, out, "embedding-backends.md")
}

// Index on BGE-M3 -> active, one-line confirmation + the ask flow. Must NOT
// trip the degraded-recall branch (guards a regex that accidentally matches).
func TestHealthHook_BGEM3ShowsActive(t *testing.T) {
	stub := stubVaultmind(t, "Embeddings: dense 50/50 (bge-m3), sparse 50/50, colbert 50/50")
	out, _ := runHealthHook(t, projectWithVault(t), stub)
	assert.Contains(t, out, "full BGE-M3 hybrid")
	assert.Contains(t, out, "vaultmind ask")
	assert.NotContains(t, out, "degraded")
}

// A mixed-model index has a MiniLM fraction running degraded 2-lane recall.
// doctor renders it as "(mixed)" + "Partial BGE-M3 coverage" — neither matches
// the (minilm)/(bge-m3) literals, so before the fix it fell through to the
// generic nudge and the partial degradation went unspoken (review finding).
func TestHealthHook_MixedModelTreatedAsPartiallyDegraded(t *testing.T) {
	doctorOut := "Embeddings: dense 78/78 (mixed), sparse 31/78, colbert 31/78\n" +
		"  mixed-model state: 47 minilm, 31 bge-m3\n" +
		"⚠ Partial BGE-M3 coverage: 47 note(s) missing sparse, 47 missing colbert"
	out, _ := runHealthHook(t, projectWithVault(t), stubVaultmind(t, doctorOut))
	assert.Contains(t, strings.ToLower(out), "partially degraded")
	assert.Contains(t, out, "vaultmind index --embed")
	assert.NotContains(t, out, "full BGE-M3 hybrid", "a mixed index must not read as fully healthy")
}

// A doctor FAILURE (bad vault, panic, corrupt DB) must surface its reason on
// stdout — not be laundered into a 'Try: vaultmind ask' nudge that fails the
// same way. Always exit 0 (the hook must never wedge a session). Review finding.
func TestHealthHook_DoctorFailureSurfacedNotLaundered(t *testing.T) {
	stub := stubVaultmindFailing(t, `Error: vault path "x" does not exist or is not a directory`, 1)
	out, errOut := runHealthHook(t, projectWithVault(t), stub)
	assert.Contains(t, out, "doctor` failed")
	assert.Contains(t, out, "does not exist", "the real doctor error must reach the adopter")
	assert.NotContains(t, out, "Try: vaultmind ask", "a failure must not render as a query suggestion")
	assert.Empty(t, strings.TrimSpace(errOut), "the message must be on stdout (SessionStart surfaces stdout)")
}

// doctor SUCCEEDS but prints an unrecognized shape (older binary) -> minimal,
// honest generic nudge. The safety-net branch, previously untested.
func TestHealthHook_UnrecognizedDoctorOutputFallsBackMinimally(t *testing.T) {
	out, _ := runHealthHook(t, projectWithVault(t), stubVaultmind(t, "some future doctor format"))
	assert.Contains(t, out, "Try: vaultmind ask")
}

// VAULTMIND_VAULT override wins over conventional-dir detection — this is what
// the parameterized settings stanza bakes in for an adopter with a custom vault.
func TestHealthHook_RespectsVaultmindVaultOverride(t *testing.T) {
	override := t.TempDir() // outside CLAUDE_PROJECT_DIR, no conventional dir
	out, _ := runHealthHook(t, t.TempDir(), "", "VAULTMIND_VAULT="+override)
	assert.Contains(t, out, override, "the nudge must name the overridden vault path")
	assert.Contains(t, out, "not installed")
}

// Persona adopters ship vaultmind-identity/, not vaultmind-vault/ — the second
// detection arm. Previously every test used vaultmind-vault.
func TestHealthHook_DetectsVaultmindIdentity(t *testing.T) {
	dir := projectWithVaultNamed(t, "vaultmind-identity")
	out, _ := runHealthHook(t, dir, "")
	assert.Contains(t, out, "vaultmind-identity")
	assert.Contains(t, out, "not installed")
}
