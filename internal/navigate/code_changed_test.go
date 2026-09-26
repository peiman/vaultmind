package navigate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo makes a repository whose commits carry the dates the test gives,
// so "after the note" is decided by the history, not by the test's timing.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "", "init", "-q")
	return dir
}

func commitFile(t *testing.T, repo, rel, content, date, subject string) {
	t.Helper()
	abs := filepath.Join(repo, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	runGit(t, repo, "", "add", rel)
	runGit(t, repo, date, "commit", "-q", "-m", subject)
}

func runGit(t *testing.T, dir, date string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = withoutGitLocation(os.Environ())
	if date != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestMarkCodeChanged_FlagsCodeCommittedAfterTheNote(t *testing.T) {
	vault, code := gitRepo(t), gitRepo(t)
	commitFile(t, code, "cmd/run.go", "v1", "2026-09-01T10:00:00Z", "feat: first")
	commitFile(t, vault, "concepts/run.md", "note", "2026-09-10T10:00:00Z", "vault: note about run")
	commitFile(t, code, "cmd/run.go", "v2", "2026-09-20T10:00:00Z", "fix: retry the run")
	commitFile(t, code, "cmd/run.go", "v3", "2026-09-25T10:00:00Z", "feat: run in parallel")

	notes := []Note{{ID: "concept-run", Path: "concepts/run.md"}}
	MarkCodeChanged(notes, vault, CodeFile{Rel: "cmd/run.go", Root: code})

	require.NotNil(t, notes[0].CodeChanged)
	assert.Equal(t, 2, notes[0].CodeChanged.Commits, "only the commits after the note count")
	assert.Equal(t, "2026-09-25", notes[0].CodeChanged.Date)
	assert.Equal(t, "feat: run in parallel", notes[0].CodeChanged.Latest)
}

func TestMarkCodeChanged_LeavesANoteNewerThanTheCode(t *testing.T) {
	vault, code := gitRepo(t), gitRepo(t)
	commitFile(t, code, "cmd/run.go", "v1", "2026-09-01T10:00:00Z", "feat: first")
	commitFile(t, vault, "concepts/run.md", "note", "2026-09-10T10:00:00Z", "vault: note")

	notes := []Note{{ID: "concept-run", Path: "concepts/run.md"}}
	MarkCodeChanged(notes, vault, CodeFile{Rel: "cmd/run.go", Root: code})
	assert.Nil(t, notes[0].CodeChanged)
}

// Without history on either side there is nothing to compare: a note not yet
// committed was just written, and a file outside a repository has no commits.
func TestMarkCodeChanged_SaysNothingWithoutHistory(t *testing.T) {
	vault, code := gitRepo(t), gitRepo(t)
	commitFile(t, code, "cmd/run.go", "v1", "2026-09-20T10:00:00Z", "feat: first")
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "concepts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "concepts", "new.md"), []byte("n"), 0o644))

	notes := []Note{{ID: "concept-new", Path: "concepts/new.md"}}
	MarkCodeChanged(notes, vault, CodeFile{Rel: "cmd/run.go", Root: code})
	assert.Nil(t, notes[0].CodeChanged, "an uncommitted note is as new as it gets")

	notes = []Note{{ID: "concept-new", Path: "concepts/new.md"}}
	MarkCodeChanged(notes, vault, CodeFile{Rel: "run.go"})
	assert.Nil(t, notes[0].CodeChanged, "a file outside a repository has no history")
}

// A rebased commit keeps its old author date but gets a new committer date.
// Committer time decides, so committer date is what is shown — never a date
// older than the note it outdates.
func TestMarkCodeChanged_ShowsTheCommitterDateOfARebasedCommit(t *testing.T) {
	vault, code := gitRepo(t), gitRepo(t)
	commitFile(t, code, "cmd/run.go", "v1", "2026-09-01T10:00:00Z", "feat: first")
	commitFile(t, vault, "concepts/run.md", "note", "2026-09-10T10:00:00Z", "vault: note")
	require.NoError(t, os.WriteFile(filepath.Join(code, "cmd/run.go"), []byte("v2"), 0o644))
	runGit(t, code, "", "add", "cmd/run.go")
	cmd := exec.Command("git", "-C", code, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "fix: rebased")
	cmd.Env = append(withoutGitLocation(os.Environ()), "GIT_AUTHOR_DATE=2026-09-05T10:00:00Z", "GIT_COMMITTER_DATE=2026-09-15T10:00:00Z")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	notes := []Note{{ID: "concept-run", Path: "concepts/run.md"}}
	MarkCodeChanged(notes, vault, CodeFile{Rel: "cmd/run.go", Root: code})
	require.NotNil(t, notes[0].CodeChanged)
	assert.Equal(t, "2026-09-15", notes[0].CodeChanged.Date)
}
