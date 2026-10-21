package onboard_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The README Quickstart and init's next steps share one list of commands.
// The test reads the README from the repo root so the two cannot drift.
func TestFirstSteps_MatchTheREADMEQuickstart(t *testing.T) {
	body, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	block := quickstartBlock(t, string(body))

	// Compare the complete bash block, so extra or reordered steps also fail.
	_, code, ok := strings.Cut(block, "```bash\n")
	require.True(t, ok)
	code, _, ok = strings.Cut(code, "```")
	require.True(t, ok)
	want := append([]string{"vaultmind init " + onboard.QuickstartVault}, onboard.FirstSteps(onboard.QuickstartVault)...)
	assert.Equal(t, want, strings.Split(strings.TrimSpace(code), "\n"))
	prose := strings.ReplaceAll(block, "`", "")
	assert.Contains(t, prose, onboard.ShellExpandSentence)
	assert.Contains(t, prose, onboard.InitFlagsLine)
	assert.Contains(t, prose, "drop docs if the project has no docs/")
	assert.Contains(t, prose, "zsh aborts the whole command")
	assert.Contains(t, prose, "if the project has no root Markdown, leave out *.md")
	assert.Contains(t, prose, "Re-running the same command picks up changes")
	assert.Contains(t, prose, "delete or rename")
	assert.Contains(t, prose, "<vault>/imported/")
	assert.Contains(t, prose, "vaultmind index --vault <vault>")
}

// The install section names the archives the release actually publishes.
func TestREADMEInstall_NamesThePublishedArchives(t *testing.T) {
	body, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	install := installBlock(t, string(body))
	// The README names the newest release's archives, so it can't silently go stale.
	latest := latestReleasedVersion(t)

	for _, want := range []string{
		"vaultmind_v" + latest + "_darwin_arm64_ort.tar.gz",
		"vaultmind_v" + latest + "_linux_amd64_ort.tar.gz",
		"darwin_arm64",
		"linux_amd64",
		"libonnxruntime",
		"PATH",
		"vaultmind_<version>_<os>_<arch>.tar.gz",
		"no `v`",
		"go install github.com/peiman/vaultmind@latest",
		"vaultmind index --embed",
		"2.3 GB",
		"74 notes",
		"133 seconds",
		"2.7 GB",
		"Apple Silicon Mac",
	} {
		assert.Contains(t, install, want, want)
	}
}

func quickstartBlock(t *testing.T, readme string) string {
	t.Helper()
	return section(t, readme, "## Quickstart")
}

func installBlock(t *testing.T, readme string) string {
	t.Helper()
	return section(t, readme, "## Install")
}

func section(t *testing.T, readme, heading string) string {
	t.Helper()
	i := strings.Index(readme, heading)
	require.GreaterOrEqual(t, i, 0, heading)
	rest := readme[i+len(heading):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// latestReleasedVersion is the first dated "## [X.Y.Z] - date" heading in CHANGELOG.md.
func latestReleasedVersion(t *testing.T) string {
	t.Helper()
	changelog, err := os.ReadFile("../../CHANGELOG.md")
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\] - `).FindSubmatch(changelog)
	require.NotNil(t, m, "no dated release heading in CHANGELOG.md")
	return string(m[1])
}
