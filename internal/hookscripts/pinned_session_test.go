package hookscripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sentinel is in the opening sentence. The session hook excerpts a long
// note to its opening, and that opening is what these tests require to see.
// The persona hook must still not print it.
const pinnedSentinel = "PINNED-LEDGER-SENTINEL-315"

func TestPinnedHook_BudgetDefaultMatchesTheCommand(t *testing.T) {
	want := fmt.Sprintf("${VAULTMIND_PINNED_BUDGET:-%d}", memory.SessionPinnedBudget)
	// The case arm is the non-numeric fallback. A second literal here would
	// let the default and the fallback drift.
	fallback := fmt.Sprintf("PINNED_BUDGET=%d ;;", memory.SessionPinnedBudget)
	for _, name := range []string{"vaultmind-health.sh"} {
		body, ok := hookscripts.Get(name)
		require.True(t, ok, name)
		script := string(body)
		assert.Contains(t, script, want)
		assert.Contains(t, script, fallback)
	}
}

func TestPinnedHook_ExcerptDefaultMatchesTheCommand(t *testing.T) {
	want := fmt.Sprintf("${VAULTMIND_PINNED_EXCERPT:-%d}", memory.SessionPinnedExcerpt)
	fallback := fmt.Sprintf("PINNED_EXCERPT=%d ;;", memory.SessionPinnedExcerpt)
	for _, name := range []string{"vaultmind-health.sh"} {
		body, ok := hookscripts.Get(name)
		require.True(t, ok, name)
		script := string(body)
		assert.Contains(t, script, want)
		assert.Contains(t, script, fallback)
	}
}

func TestHealthHook_PinnedNoteExcerpt(t *testing.T) {
	binary := pinnedTestBinary(t)
	vault := pinnedHookVault(t)
	project := t.TempDir()
	out, _ := runHealthHook(t, project, filepath.Dir(binary),
		"VAULTMIND_VAULT="+vault,
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_NO_UPDATE_CHECK=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
		"HOME="+t.TempDir(),
	)
	assert.Contains(t, out, pinnedSentinel)
	assert.Contains(t, out, "work-ledger")
}

// An older binary has no `note pinned`. Cobra writes the usage screen to
// stdout and exits 1. `$(... || true)` keeps that stdout, so the screen
// landed in the session. A failed call contributes nothing.
func TestHealthHook_FailedPinnedCommandDoesNotLeakStdout(t *testing.T) {
	bin := t.TempDir()
	body := "#!/bin/bash\n" +
		"if [ \"$1\" = doctor ]; then printf '%s\\n' " + shellSingleQuote(healthBGEM3) + "; exit 0; fi\n" +
		"if [ \"$1 $2\" = \"note pinned\" ]; then printf '%s\\n' 'STUB-USAGE-SCREEN'; exit 1; fi\n" +
		"exit 0\n"
	hookscripts.InstallStub(t, bin, "vaultmind", body)
	out, _ := runHealthHook(t, projectWithVault(t), bin)
	assert.NotContains(t, out, "STUB-USAGE-SCREEN")
	assert.Contains(t, out, "full BGE-M3", "the health line still arrives")
}

// Three vaults each spending the budget would triple it. One call, one budget.
func TestHealthHook_PinnedBudgetIsOnceAcrossVaults(t *testing.T) {
	binary := pinnedTestBinary(t)
	first := pinnedHookVaultWith(t, map[string]string{
		"z.md": pinnedHookNote("note-z", "Zebra", exactTokenBodyHook("ZEBRA-BODY")),
	})
	second := pinnedHookVaultWith(t, map[string]string{
		"a.md": pinnedHookNote("note-a", "Alpha", exactTokenBodyHook("ALPHA-BODY")),
	})
	out, _ := runHealthHook(t, t.TempDir(), filepath.Dir(binary),
		"VAULTMIND_VAULT="+first,
		"VAULTMIND_VAULTS="+first+","+second,
		"VAULTMIND_PINNED_BUDGET=10",
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_NO_UPDATE_CHECK=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
		"HOME="+t.TempDir(),
	)
	pinned := pinnedBlock(t, out)
	assert.Equal(t, 1, strings.Count(pinned, "PINNED —"), "one block for the session, not one per vault")
	assert.Contains(t, pinned, "ZEBRA-BODY")
	assert.NotContains(t, pinned, "ALPHA-BODY")
	assert.Contains(t, pinned, "10-token budget")
}

// A note longer than the budget used to be named and not shown. The hook
// excerpts it, so the opening still arrives.
func TestHealthHook_LongPinnedNoteIsExcerpted(t *testing.T) {
	binary := pinnedTestBinary(t)
	body := "PINNED-OPENING-SENTINEL. " + strings.Repeat("a", 12000) + " PINNED-TAIL-SENTINEL"
	require.GreaterOrEqual(t, memory.EstimateTokens(body), 3000)
	vault := pinnedHookVaultWith(t, map[string]string{
		"ledger.md": pinnedHookNote("work-ledger", "Work Ledger", body),
	})
	out, _ := runHealthHook(t, t.TempDir(), filepath.Dir(binary),
		"VAULTMIND_VAULT="+vault,
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_NO_UPDATE_CHECK=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
		"HOME="+t.TempDir(),
	)
	pinned := pinnedBlock(t, out)
	assert.Contains(t, pinned, "PINNED-OPENING-SENTINEL")
	assert.NotContains(t, pinned, "PINNED-TAIL-SENTINEL")
}

// pinnedBlock is the session's pinned loader, after the map. The map's
// one-line descriptions quote the same opening, so a check of the whole
// hook output cannot tell a loaded note from a listed one.
func pinnedBlock(t *testing.T, out string) string {
	t.Helper()
	idx := strings.Index(out, "PINNED —")
	require.NotEqual(t, -1, idx, "pinned notes were not loaded:\n%s", out)
	return out[idx:]
}

// A non-numeric cap used to be handed to --budget, the command failed, and
// the session started with no pins. Same fallback as the map cap.
func TestHealthHook_NonNumericPinnedLimitsFallBack(t *testing.T) {
	binary := pinnedTestBinary(t)
	vault := pinnedHookVault(t)
	out, _ := runHealthHook(t, t.TempDir(), filepath.Dir(binary),
		"VAULTMIND_VAULT="+vault,
		"VAULTMIND_PINNED_BUDGET=lots",
		"VAULTMIND_PINNED_EXCERPT=nope",
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_NO_UPDATE_CHECK=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
		"HOME="+t.TempDir(),
	)
	assert.Contains(t, out, pinnedSentinel)
}

// The health hook runs under every profile and covers every vault, and the
// persona hook runs beside it in a full install. Pins print once: from the
// health hook only.
func TestPersonaHook_LeavesPinnedNotesToTheHealthHook(t *testing.T) {
	binary := pinnedTestBinary(t)
	vault := pinnedHookVault(t)
	project, env := installPersonaHook(t, "#!/bin/bash\nexit 0\n")
	// installPersonaHook puts a stub first on PATH. Replace it with the real binary.
	env = replacePathBin(env, filepath.Dir(binary))
	env = append(env,
		"LOAD_PERSONA_VAULT="+vault,
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_NO_UPDATE_CHECK=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
	)
	out := runPersonaHook(t, project, env)
	assert.Contains(t, out, "IDENTITY CONTEXT:")
	assert.NotContains(t, out, pinnedSentinel, "the health hook prints pins; printing them here too doubles them")
}

var (
	pinnedBinOnce sync.Once
	pinnedBinPath string
	pinnedBinErr  error
)

func pinnedTestBinary(t *testing.T) string {
	t.Helper()
	pinnedBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vm-pinned-bin-")
		if err != nil {
			pinnedBinErr = err
			return
		}
		pinnedBinPath = filepath.Join(dir, "vaultmind")
		build := exec.Command("go", "build", "-tags", "dev", "-o", pinnedBinPath, ".")
		build.Dir = repoRoot(t)
		raw, err := build.CombinedOutput()
		if err != nil {
			pinnedBinErr = fmt.Errorf("%w\n%s", err, raw)
		}
	})
	require.NoError(t, pinnedBinErr)
	return pinnedBinPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func pinnedHookVault(t *testing.T) string {
	t.Helper()
	body := "The ledger opens with the current job. " + pinnedSentinel + "\n"
	return pinnedHookVaultWith(t, map[string]string{
		"who.md":    "---\nid: identity-who\ntype: concept\ntitle: Who I Am\n---\nI am the agent. Who I am is written here.\n",
		"ledger.md": pinnedHookNote("work-ledger", "Work Ledger", body),
	})
}

func pinnedHookVaultWith(t *testing.T, notes map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NotEmpty(t, notes)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte("types:\n  concept:\n    required: [title]\n"), 0o600))
	for name, content := range notes {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

func pinnedHookNote(id, title, body string) string {
	return "---\nid: " + id + "\ntype: concept\ntitle: " + title + "\npinned: true\n---\n" + body
}

// exactTokenBodyHook is 40 bytes, which EstimateTokens counts as 10.
func exactTokenBodyHook(sentinel string) string {
	return sentinel + strings.Repeat("x", 40-len(sentinel))
}

func replacePathBin(env []string, binDir string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			out = append(out, "PATH="+binDir+":/usr/bin:/bin")
			continue
		}
		out = append(out, e)
	}
	return out
}
