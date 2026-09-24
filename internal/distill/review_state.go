package distill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/parser"
)

// Which episodes still await review. Review (review.go) is what finds turning
// points; it closes the loop from episode to arc only if it happens without
// anyone having to remember it. So the judgement is recorded where it applies —
// a `reviewed` date on the episode's own frontmatter, the way a desk entry
// records the arc it became (distilled_to) — and whatever is not yet judged can
// be counted and surfaced at session start.

// ReviewedField marks an episode an agent has judged for turning points. Its
// value is the date of the judgement.
const ReviewedField = "reviewed"

const (
	episodeFileExt   = ".md"
	startedAtField   = "started_at"
	episodeDateChars = len(dateLayout)
)

// PendingEpisode is an episode that holds messages worth judging and has not
// been reviewed. File — the file name without .md — is what opens and marks
// it; ID is the id the episode carries, for display. Capture writes them equal,
// but a renamed or imported episode can differ, and treating the id as a file
// name would jam the queue on it or mark another file.
type PendingEpisode struct {
	File     string `json:"file"`
	ID       string `json:"id"`
	Date     string `json:"date"`
	Messages int    `json:"messages"`
}

// PendingReviews lists the episodes in dir awaiting review, oldest first. An
// episode with nothing to judge (only machine text or one-word replies) is not
// pending: there is nothing a reviewer could find in it. A missing dir means a
// vault that captures no sessions, which has nothing pending.
//
// An episode that cannot be read is skipped and named in the diagnostics
// rather than failing the scan — one bad file must not block the whole queue
// (the same rule ScanEpisodes follows). The error is for the listing itself.
func PendingReviews(dir string) ([]PendingEpisode, []string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+episodeFileExt))
	if err != nil {
		return nil, nil, fmt.Errorf("listing episodes in %s: %w", dir, err)
	}
	out, diags := []PendingEpisode{}, []string{}
	for _, p := range paths {
		pe, pending, err := pendingEpisode(p)
		if err != nil {
			diags = append(diags, fmt.Sprintf("skipped %s: %v", filepath.Base(p), err))
			continue
		}
		if pending {
			out = append(out, pe)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].File < out[j].File
	})
	return out, diags, nil
}

// pendingEpisode reads one episode and reports whether it awaits review.
func pendingEpisode(path string) (PendingEpisode, bool, error) {
	fm, err := episodeFrontmatter(path)
	if err != nil {
		return PendingEpisode{}, false, err
	}
	if stringField(fm, ReviewedField) != "" {
		return PendingEpisode{}, false, nil
	}
	ep, err := ParseEpisodeFile(path)
	if err != nil {
		return PendingEpisode{}, false, err
	}
	n := len(BuildReview(ep).Messages)
	if n == 0 {
		return PendingEpisode{}, false, nil
	}
	return PendingEpisode{
		File: strings.TrimSuffix(filepath.Base(path), episodeFileExt),
		ID:   ep.ID, Date: episodeDate(fm), Messages: n,
	}, true, nil
}

// MarkReviewed records on the episode id in dir that it was reviewed on date.
// id may carry the .md extension; it never names a path outside dir.
func MarkReviewed(dir, id, date string) error {
	name := strings.TrimSuffix(strings.TrimSpace(id), episodeFileExt)
	if name == "" || name != filepath.Base(name) {
		return fmt.Errorf("%q is not an episode id", id)
	}
	path := filepath.Join(dir, name+episodeFileExt)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no episode %q in %s", name, dir)
		}
		return err
	}
	return mutation.SetFileKey(path, ReviewedField, date)
}

func episodeFrontmatter(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- episode path from a glob of the vault's episodes // nosemgrep: go-path-traversal
	if err != nil {
		return nil, fmt.Errorf("reading episode: %w", err)
	}
	fm, _, err := parser.ExtractFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err) // the caller names the file
	}
	return fm, nil
}

// episodeDate is the day the episode started, from its started_at timestamp.
func episodeDate(fm map[string]interface{}) string {
	d := stringField(fm, startedAtField)
	if len(d) > episodeDateChars {
		d = d[:episodeDateChars]
	}
	return d
}
