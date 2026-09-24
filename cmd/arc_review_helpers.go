package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/envelope"
)

const (
	arcReviewEnvelope = "arc-candidates-review"
	episodesSubdir    = "episodes"
	episodeExt        = ".md"
)

// arcReviewResult is the JSON payload of `arc candidates --review`.
type arcReviewResult struct {
	Sessions []distill.ReviewSession `json:"sessions"`
}

// runArcReview writes the review list for the named episodes, or for the most
// recent one when none are named. See internal/distill/review.go for why review
// replaces guessing.
func runArcReview(w io.Writer, vaultPath, episodes string, jsonOut bool) error {
	dir := filepath.Join(vaultPath, episodesSubdir)
	paths, err := reviewEpisodePaths(dir, episodes)
	if err != nil {
		return err
	}
	res := arcReviewResult{Sessions: []distill.ReviewSession{}}
	for _, p := range paths {
		ep, perr := distill.ParseEpisodeFile(p)
		if perr != nil {
			return perr
		}
		res.Sessions = append(res.Sessions, distill.BuildReview(ep))
	}
	if jsonOut {
		env := envelope.OK(arcReviewEnvelope, res)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(w).Encode(env)
	}
	return writeArcReview(w, res)
}

// reviewEpisodePaths resolves comma-separated episode ids to files, or picks the
// most recently written episode when none are named.
func reviewEpisodePaths(dir, episodes string) ([]string, error) {
	if strings.TrimSpace(episodes) != "" {
		var out []string
		for _, id := range strings.Split(episodes, ",") {
			id = strings.TrimSuffix(strings.TrimSpace(id), episodeExt)
			if id == "" {
				continue
			}
			p := filepath.Join(dir, filepath.Base(id)+episodeExt)
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("no episode %q in %s", id, dir)
			}
			out = append(out, p)
		}
		return out, nil
	}
	all, err := filepath.Glob(filepath.Join(dir, "*"+episodeExt))
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no episodes in %s — capture sessions first (hooks install wires capture at session end)", dir)
	}
	sort.Slice(all, func(i, j int) bool { return modTime(all[i]) > modTime(all[j]) })
	return all[:1], nil
}

func modTime(p string) int64 {
	if fi, err := os.Stat(p); err == nil {
		return fi.ModTime().UnixNano()
	}
	return 0
}

func writeArcReview(w io.Writer, res arcReviewResult) error {
	b := &strings.Builder{}
	b.WriteString("Which of the person's messages changed how you understand or approach something?\n")
	b.WriteString("Pick a handful per session. Routine requests and approvals are not turning points.\n")
	for _, s := range res.Sessions {
		fmt.Fprintf(b, "\n=== %s (%d messages) ===\n", s.EpisodeID, len(s.Messages))
		for _, m := range s.Messages {
			before := m.AgentBefore
			if before == "" {
				before = "(start of session)"
			}
			fmt.Fprintf(b, "[%d] agent: %s\n    person: %s\n", m.Index, before, m.Person)
		}
	}
	b.WriteString("\nFor each turning point, write a desk entry (a note with `type: journal`) that cites\n")
	b.WriteString("the episode and message number. Arcs come later, by hand — see principle-how-to-write-arcs.\n")
	_, err := io.WriteString(w, b.String())
	return err
}
