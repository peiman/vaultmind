package query

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/peiman/vaultmind/internal/index"
)

// DoctorDraftNote is one note waiting to be accepted. AgeDays is whole days
// since the file's working-tree mtime (the index mtime when the file is gone).
type DoctorDraftNote struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	AgeDays int    `json:"age_days"`
}

// DoctorDrafts lists notes whose status is exactly "draft".
type DoctorDrafts struct {
	Count int               `json:"count"`
	Notes []DoctorDraftNote `json:"notes"`
}

// DoctorUncommittedNote is a markdown file git still has as untracked or
// modified, older than the doctor's threshold.
type DoctorUncommittedNote struct {
	Path    string `json:"path"`
	AgeDays int    `json:"age_days"`
	State   string `json:"state"`
}

// DoctorUncommitted is the uncommitted-notes check. Skipped is set when the
// vault is not a git repository: an empty list is then not a clean result.
type DoctorUncommitted struct {
	ThresholdDays int                     `json:"threshold_days"`
	Count         int                     `json:"count"`
	Notes         []DoctorUncommittedNote `json:"notes"`
	Skipped       bool                    `json:"skipped"`
	Reason        string                  `json:"reason,omitempty"`
}

// uncommittedSkipReason is the text and JSON message when the check does not run.
const uncommittedSkipReason = "vault is not a git repository"

// ListDraftNotes returns every indexed note whose status is exactly draft,
// oldest path first. Age is the working-tree mtime, then the indexed mtime.
func ListDraftNotes(db *index.DB, vaultPath string, now time.Time) (DoctorDrafts, error) {
	rows, err := db.Query(`SELECT id, path, mtime FROM notes WHERE status = ? ORDER BY path`, index.StatusDraft)
	if err != nil {
		return DoctorDrafts{}, fmt.Errorf("listing drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := DoctorDrafts{Notes: []DoctorDraftNote{}}
	for rows.Next() {
		var n DoctorDraftNote
		var mtime int64
		if err := rows.Scan(&n.ID, &n.Path, &mtime); err != nil {
			return DoctorDrafts{}, fmt.Errorf("listing drafts: %w", err)
		}
		n.AgeDays = ageDays(fileMTime(vaultPath, n.Path, mtime), now)
		out.Notes = append(out.Notes, n)
	}
	if err := rows.Err(); err != nil {
		return DoctorDrafts{}, fmt.Errorf("listing drafts: %w", err)
	}
	out.Count = len(out.Notes)
	return out, nil
}

// CheckUncommittedNotes reports .md files git sees as untracked or modified
// whose working-tree mtime is strictly older than days. A vault that is not a
// git repository is skipped with a reason, not reported as clean.
func CheckUncommittedNotes(vaultPath string, days int, now time.Time) (DoctorUncommitted, error) {
	out := DoctorUncommitted{ThresholdDays: days, Notes: []DoctorUncommittedNote{}}
	repo, err := gogit.PlainOpenWithOptions(vaultPath, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			out.Skipped = true
			out.Reason = uncommittedSkipReason
			return out, nil
		}
		return DoctorUncommitted{}, fmt.Errorf("uncommitted notes: opening repository for %q: %w", vaultPath, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return DoctorUncommitted{}, fmt.Errorf("uncommitted notes: worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return DoctorUncommitted{}, fmt.Errorf("uncommitted notes: status: %w", err)
	}
	notes := staleUncommittedMarkdown(wt.Filesystem.Root(), vaultPath, status, days, now)
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	out.Notes = notes
	out.Count = len(notes)
	return out, nil
}

func staleUncommittedMarkdown(root, vaultPath string, status gogit.Status, days int, now time.Time) []DoctorUncommittedNote {
	notes := []DoctorUncommittedNote{}
	for gitPath, st := range status {
		state := markdownGitState(st)
		if state == "" || !strings.HasSuffix(gitPath, ".md") {
			continue
		}
		rel, ok := pathInsideVault(root, vaultPath, gitPath)
		if !ok {
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(gitPath)))
		if err != nil || info.IsDir() {
			continue
		}
		if !noteOlderThan(info.ModTime(), now, days) {
			continue
		}
		notes = append(notes, DoctorUncommittedNote{
			Path:    rel,
			AgeDays: ageDays(info.ModTime(), now),
			State:   state,
		})
	}
	return notes
}

// markdownGitState classifies a git status as untracked or modified.
// Added, renamed and deleted files are not "left sitting uncommitted".
func markdownGitState(st *gogit.FileStatus) string {
	if st == nil {
		return ""
	}
	if st.Staging == gogit.Untracked && st.Worktree == gogit.Untracked {
		return "untracked"
	}
	if st.Worktree == gogit.Modified || st.Staging == gogit.Modified {
		return "modified"
	}
	return ""
}

// pathInsideVault returns gitPath relative to the vault when the file is inside it.
func pathInsideVault(root, vaultPath, gitPath string) (string, bool) {
	rootAbs, err := resolvedPath(root)
	if err != nil {
		return "", false
	}
	vaultAbs, err := resolvedPath(vaultPath)
	if err != nil {
		return "", false
	}
	relVault, err := filepath.Rel(rootAbs, vaultAbs)
	if err != nil {
		return "", false
	}
	relVault = filepath.ToSlash(relVault)
	gitPath = filepath.ToSlash(gitPath)
	if relVault == "." {
		return gitPath, true
	}
	prefix := relVault + "/"
	if !strings.HasPrefix(gitPath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(gitPath, prefix), true
}

// resolvedPath cleans a path and follows symlinks. Temp dirs on macOS are
// reached through /var, which is /private/var; git's worktree root and the
// vault path must name the same directory or every file looks outside the vault.
func resolvedPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, symErr := filepath.EvalSymlinks(abs)
	if symErr == nil {
		return resolved, nil
	}
	// A missing leaf still has an absolute form. Comparing two paths must not
	// fail the whole check because one side does not exist yet.
	return abs, nil
}

func fileMTime(vaultPath, rel string, fallbackUnix int64) time.Time {
	info, err := os.Stat(filepath.Join(vaultPath, rel))
	if err != nil {
		return time.Unix(fallbackUnix, 0)
	}
	return info.ModTime()
}

func ageDays(mtime, now time.Time) int {
	days := int(now.Sub(mtime) / (24 * time.Hour))
	if days < 0 {
		return 0
	}
	return days
}

// noteOlderThan reports whether mtime is strictly older than days.
// A file touched exactly N days ago is still inside the window.
func noteOlderThan(mtime, now time.Time, days int) bool {
	return now.Sub(mtime) > time.Duration(days)*24*time.Hour
}
