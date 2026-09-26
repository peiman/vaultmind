package navigate

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// CodeChange says the code a note covers was committed to after the note was.
// It is a prompt to check the note still holds, not a verdict: a broad glob
// changes for reasons that leave the note true.
type CodeChange struct {
	Commits int    `json:"commits"`
	Date    string `json:"date"`
	Latest  string `json:"latest"`
	// Summary is the sentence every surface shows — the text map and the
	// code-map hook — so they cannot word the warning differently.
	Summary string `json:"summary"`
}

// MarkCodeChanged sets CodeChanged on each note whose file was last committed
// before f was. Commit times on both sides, never file times: a checkout
// rewrites every mtime and would make every note look current. Without history
// on either side — a note not yet committed, a file outside a repository —
// there is nothing to compare, and the note is left unmarked.
func MarkCodeChanged(notes []Note, vaultRoot string, f CodeFile) {
	if f.Root == "" {
		return
	}
	for i := range notes {
		noteTime, ok := lastCommitTime(vaultRoot, notes[i].Path)
		if !ok {
			continue
		}
		notes[i].CodeChanged = commitsAfter(f.Root, f.Rel, noteTime)
	}
}

// lastCommitTime is when path was last committed in the repository at dir.
func lastCommitTime(dir, path string) (int64, bool) {
	out, err := runGitLog(dir, "-1", "--format=%ct", "--", path)
	if err != nil || out == "" {
		return 0, false
	}
	ts, err := strconv.ParseInt(out, 10, 64)
	return ts, err == nil
}

// commitsAfter summarises the commits to rel in root made after since.
func commitsAfter(root, rel string, since int64) *CodeChange {
	out, err := runGitLog(root, "--format=%ct%x09%as%x09%s", "--", rel)
	if err != nil || out == "" {
		return nil
	}
	var change *CodeChange
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		ts, perr := strconv.ParseInt(parts[0], 10, 64)
		if perr != nil || ts <= since {
			continue
		}
		if change == nil {
			change = &CodeChange{Date: parts[1], Latest: parts[2]}
		}
		change.Commits++
	}
	if change != nil {
		change.Summary = summarize(change)
	}
	return change
}

func summarize(c *CodeChange) string {
	commits := "commits"
	if c.Commits == 1 {
		commits = "commit"
	}
	return fmt.Sprintf("code changed since this note: %d %s, latest %s %q — check it still holds",
		c.Commits, commits, c.Date, c.Latest)
}

// runGitLog runs `git log` in dir, without the inherited repository location
// (see gitLocationVars): under a git hook it points at the calling repository,
// and every answer would be about the wrong one.
func runGitLog(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "log"}, args...)...) //nolint:gosec // G204: dir and paths come from the vault index and the file being read, passed as separate args after --
	cmd.Env = withoutGitLocation(os.Environ())
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitLocationVars are what git exports to a hook to say which repository it is
// in. Inherited, they point every git command at that repository instead of
// the one named with -C — a hook's GIT_INDEX_FILE once made a test's
// `git add` write into the calling worktree's index.
var gitLocationVars = []string{
	"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_COMMON_DIR=", "GIT_PREFIX=",
	"GIT_OBJECT_DIRECTORY=", "GIT_ALTERNATE_OBJECT_DIRECTORIES=",
}

// withoutGitLocation drops gitLocationVars. Set to empty they are still set,
// and git refuses an empty path, so they are removed.
func withoutGitLocation(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !hasAnyPrefix(kv, gitLocationVars) {
			out = append(out, kv)
		}
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
