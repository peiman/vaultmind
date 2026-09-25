package hookscripts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Before a commit the reach hook asks the vault about THIS commit, taken from
// its message, not the same fixed sentence every time. A fixed query returns
// the same notes before every commit, and a note seen eight times a day stops
// being read (measured 2026-09-25: the same three notes on every commit).
// The fixed query stays for a commit with no message on the command line.

const fixedCommitQuery = "commit discipline"

func commitReach(t *testing.T, cmd string) string {
	t.Helper()
	h := newHookEnv(t, echoStub)
	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), bashPayload(cmd))
	return out
}

func TestReachCommit_AsksAboutTheCommitSubject(t *testing.T) {
	out := commitReach(t, `git commit -m "fix(hooks): a dry run writes nothing"`)
	assert.Contains(t, out, "a dry run writes nothing", "the query is the commit's own subject")
	assert.NotContains(t, out, fixedCommitQuery)
}

func TestReachCommit_UsesOnlyTheSubjectLine(t *testing.T) {
	out := commitReach(t, "cd repo && git add -A && git commit -q -m \"feat: explore mode\n\nA long body that is not the subject.\"")
	assert.Contains(t, out, "explore mode")
	assert.NotContains(t, out, "long body", "the body is not the query")
}

func TestReachCommit_ReadsAHeredocMessage(t *testing.T) {
	out := commitReach(t, "git commit -m \"$(cat <<'EOF'\nfix: sidecar keeps one record per session\n\nbody\nEOF\n)\"")
	assert.Contains(t, out, "sidecar keeps one record per session")
}

func TestReachCommit_ReadsCombinedFlagsAndAGitDir(t *testing.T) {
	assert.Contains(t, commitReach(t, `git commit -qam "stage and commit"`), "stage and commit")
	assert.Contains(t, commitReach(t, `git -C /tmp/repo commit -m "commit elsewhere"`), "commit elsewhere")
}

func TestReachCommit_WithoutAMessageKeepsTheFixedQuery(t *testing.T) {
	assert.Contains(t, commitReach(t, "git commit"), fixedCommitQuery)
}

// Only a real commit fires. The words "git commit" inside another command — a
// grep, an echo — used to bring up the commit notes too.
func TestReachCommit_MentioningGitCommitIsNotACommit(t *testing.T) {
	assert.Empty(t, commitReach(t, `grep -rn "git commit" internal/`))
	assert.Empty(t, commitReach(t, `echo "run git commit later"`))
	assert.Empty(t, commitReach(t, `git commit-tree abc123 -m plumbing`), "commit-tree is not commit")
}
