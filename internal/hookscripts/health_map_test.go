package hookscripts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At session start the health hook shows the map of the project's knowledge
// vault — what it holds, by folder — so the agent starts knowing what is there
// to ask about instead of having to guess a query.

const healthBGEM3 = "Embeddings: dense 50/50 (bge-m3), sparse 50/50, colbert 50/50"

// stubVaultmindWithTree answers doctor with a full-recall tier and tree with
// treeLines lines, logging every call's arguments to the returned log file.
// treeCode is tree's exit status (non-zero plays an older binary without tree).
func stubVaultmindWithTree(t *testing.T, treeLines, treeCode int) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(bin, "calls.log")
	body := fmt.Sprintf("#!/bin/bash\necho \"$*\" >> %s\n"+
		"if [ \"$1\" = doctor ]; then printf '%%s\\n' %s; fi\n"+
		"if [ \"$1\" = tree ]; then\n"+
		"  [ %d -ne 0 ] && { echo 'Error: unknown command \"tree\"' >&2; exit %d; }\n"+
		"  echo 'kb — %d notes'; i=1; while [ $i -lt %d ]; do echo \"folder-$i/ (3)\"; i=$((i+1)); done\n"+
		"fi\n", shellSingleQuote(log), shellSingleQuote(healthBGEM3), treeCode, treeCode, treeLines, treeLines)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "vaultmind"), []byte(body), 0o755))
	return bin, log
}

func TestHealthMap_ShowsTheVaultMapAtSessionStart(t *testing.T) {
	bin, log := stubVaultmindWithTree(t, 4, 0)
	project := projectWithVault(t)

	out, _ := runHealthHook(t, project, bin)

	assert.Contains(t, out, "MAP", "the map is announced")
	assert.Contains(t, out, "folder-1/ (3)", "the map itself is shown")
	assert.Contains(t, out, "vaultmind note get", "and how to open what it lists")
	calls, err := os.ReadFile(log) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Contains(t, string(calls), "tree --vault "+filepath.Join(project, "vaultmind-vault")+" --depth 1",
		"an overview, not every note")
}

func TestHealthMap_ALongMapIsCappedAndSaysHowToSeeTheRest(t *testing.T) {
	bin, _ := stubVaultmindWithTree(t, 200, 0)

	out, _ := runHealthHook(t, projectWithVault(t), bin)

	assert.Contains(t, out, "folder-1/ (3)")
	assert.NotContains(t, out, "folder-150/", "a flat vault must not flood the session start")
	assert.Contains(t, out, "more lines", "the cut says there is more")
	assert.LessOrEqual(t, strings.Count(out, "folder-"), 40)
}

// A cap that is not a number falls back to the default. awk compared it as a
// string ("1" <= "lots"), so the cap was silently ignored and a flat vault
// flooded the session start.
func TestHealthMap_ANonNumericCapFallsBackToTheDefault(t *testing.T) {
	bin, _ := stubVaultmindWithTree(t, 200, 0)

	out, _ := runHealthHook(t, projectWithVault(t), bin, "VAULTMIND_MAP_MAX_LINES=lots")

	assert.Contains(t, out, "folder-1/ (3)", "the map must still be shown")
	assert.NotContains(t, out, "folder-150/", "and still be capped")
	assert.Contains(t, out, "more lines")
}

func TestHealthMap_AnOlderBinaryWithoutTreeSkipsTheMap(t *testing.T) {
	bin, _ := stubVaultmindWithTree(t, 4, 1)

	out, _ := runHealthHook(t, projectWithVault(t), bin)

	assert.NotContains(t, out, "MAP")
	assert.NotContains(t, out, "unknown command", "an older binary is not an error at session start")
	assert.Contains(t, out, "full BGE-M3", "the health line still arrives")
}

func TestHealthMap_MapsEveryConfiguredVault(t *testing.T) {
	bin, log := stubVaultmindWithTree(t, 4, 0)
	project := projectWithVault(t)
	vaults := filepath.Join(project, "vaultmind-vault") + "," + filepath.Join(project, "docs-kb")

	runHealthHook(t, project, bin, "VAULTMIND_VAULTS="+vaults)

	calls, err := os.ReadFile(log) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Contains(t, string(calls), "tree --vaults "+vaults+" --depth 1")
}
