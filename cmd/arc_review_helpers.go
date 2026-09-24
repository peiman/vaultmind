package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/envelope"
)

const (
	arcReviewEnvelope     = "arc review"
	arcReviewMarkEnvelope = "arc review mark"
	// arcReviewSkippedCode is the envelope warning for an episode that could not
	// be read and so was left out of the queue.
	arcReviewSkippedCode = "unreadable_episode"
	episodesSubdir       = "episodes"
	episodeExt           = ".md"
	// arcReviewCommand is how the review is invoked, quoted wherever a reader is
	// told to run it — the queue footer and the session-start line from `self`.
	arcReviewCommand = "vaultmind arc review --vault %s"
)

// arcReviewRequest is what `arc review` was asked to do.
type arcReviewRequest struct {
	Vault    string
	Episodes string // comma-separated ids to review; empty = oldest awaiting review
	Mark     string // comma-separated ids to record as reviewed
	JSON     bool
	Today    string // date recorded by --mark-reviewed
}

// arcReviewResult is the JSON payload of `arc review`.
type arcReviewResult struct {
	Awaiting int                     `json:"awaiting"`
	Sessions []distill.ReviewSession `json:"sessions"`
}

// arcReviewMarkResult is the JSON payload of `arc review --mark-reviewed`.
type arcReviewMarkResult struct {
	Marked   []string `json:"marked"`
	Awaiting int      `json:"awaiting"`
}

func newArcReviewRequest(vault, episodes, mark string, jsonOut bool) arcReviewRequest {
	return arcReviewRequest{Vault: vault, Episodes: episodes, Mark: mark, JSON: jsonOut,
		Today: time.Now().Format(time.DateOnly)}
}

// runArcReview writes the review list for the named episodes, or for the
// oldest one awaiting review when none are named; or records judgements. See
// internal/distill/review.go for why review replaces guessing.
func runArcReview(w io.Writer, req arcReviewRequest) error {
	dir := filepath.Join(req.Vault, episodesSubdir)
	// A vault that captures no sessions is not one whose sessions were all
	// judged; answering "nothing awaits review" would send the reader away
	// from the vault that holds them (the arc candidates typo'd-vault lesson).
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("vault %q has no episodes folder (%s): it captures no sessions — "+
			"point --vault at the vault your session hooks capture into", req.Vault, dir)
	}
	if strings.TrimSpace(req.Mark) != "" {
		return markArcReviewed(w, dir, req)
	}
	pending, skipped, err := distill.PendingReviews(dir)
	if err != nil {
		return err
	}
	paths, err := reviewEpisodePaths(dir, req.Episodes, pending)
	if err != nil {
		return err
	}
	res := arcReviewResult{Awaiting: len(pending), Sessions: []distill.ReviewSession{}}
	for _, p := range paths {
		ep, perr := distill.ParseEpisodeFile(p)
		if perr != nil {
			return perr
		}
		s := distill.BuildReview(ep)
		s.File = strings.TrimSuffix(filepath.Base(p), episodeExt)
		res.Sessions = append(res.Sessions, s)
	}
	if req.JSON {
		return writeArcReviewEnvelope(w, arcReviewEnvelope, req.Vault, res, skipped)
	}
	return writeArcReview(w, req.Vault, res, skipped)
}

// markArcReviewed records judgements. A mark that was written is reported as
// written, whatever happens after it: the recount only informs, and an
// unreadable episode elsewhere is a warning, never this command's failure.
func markArcReviewed(w io.Writer, dir string, req arcReviewRequest) error {
	res := arcReviewMarkResult{Marked: []string{}}
	for _, id := range splitIDs(req.Mark) {
		if err := distill.MarkReviewed(dir, id, req.Today); err != nil {
			if len(res.Marked) > 0 {
				return fmt.Errorf("%w (already marked: %s)", err, strings.Join(res.Marked, ","))
			}
			return err
		}
		res.Marked = append(res.Marked, strings.TrimSuffix(id, episodeExt))
	}
	pending, skipped, err := distill.PendingReviews(dir)
	if err != nil {
		skipped = append(skipped, "could not recount the queue: "+err.Error())
	}
	res.Awaiting = len(pending)
	if req.JSON {
		return writeArcReviewEnvelope(w, arcReviewMarkEnvelope, req.Vault, res, skipped)
	}
	b := &strings.Builder{}
	for _, id := range res.Marked {
		fmt.Fprintf(b, "Marked reviewed: %s\n", id)
	}
	fmt.Fprintf(b, "%s.\n", awaitingPhrase(res.Awaiting))
	writeSkipped(b, skipped)
	_, err = io.WriteString(w, b.String())
	return err
}

func writeArcReviewEnvelope(w io.Writer, name, vault string, result any, skipped []string) error {
	env := envelope.OK(name, result)
	for _, s := range skipped {
		env.AddWarning(arcReviewSkippedCode, s, "")
	}
	env.Meta.VaultPath = vault
	return json.NewEncoder(w).Encode(env)
}

func writeSkipped(b *strings.Builder, skipped []string) {
	for _, s := range skipped {
		fmt.Fprintf(b, "Not in the queue — %s\n", s)
	}
}

// reviewEpisodePaths resolves comma-separated episode ids to files, or picks
// the oldest episode awaiting review when none are named.
func reviewEpisodePaths(dir, episodes string, pending []distill.PendingEpisode) ([]string, error) {
	if strings.TrimSpace(episodes) == "" {
		if len(pending) == 0 {
			return nil, nil
		}
		return []string{filepath.Join(dir, pending[0].File+episodeExt)}, nil
	}
	var out []string
	for _, id := range splitIDs(episodes) {
		p := filepath.Join(dir, filepath.Base(strings.TrimSuffix(id, episodeExt))+episodeExt)
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("no episode %q in %s", id, dir)
		}
		out = append(out, p)
	}
	return out, nil
}

func splitIDs(s string) []string {
	var out []string
	for _, id := range strings.Split(s, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// awaitingPhrase says how many sessions await review, in a sentence.
func awaitingPhrase(n int) string {
	switch n {
	case 0:
		return "No session awaits review"
	case 1:
		return "1 session awaits review"
	default:
		return fmt.Sprintf("%d sessions await review", n)
	}
}

func writeArcReview(w io.Writer, vault string, res arcReviewResult, skipped []string) error {
	b := &strings.Builder{}
	if len(res.Sessions) == 0 {
		fmt.Fprintf(b, "%s — every captured session with the person's messages has been judged.\n",
			awaitingPhrase(res.Awaiting))
		writeSkipped(b, skipped)
		_, err := io.WriteString(w, b.String())
		return err
	}
	b.WriteString("Which of the person's messages changed how you understand or approach something?\n")
	b.WriteString("Pick a handful per session. Routine requests and approvals are not turning points.\n")
	files := make([]string, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		files = append(files, s.File)
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
	fmt.Fprintf(b, "\n%s. When you have judged this, record it so the queue moves on:\n  "+
		arcReviewCommand+" --mark-reviewed %s\n", awaitingPhrase(res.Awaiting), vault, strings.Join(files, ","))
	writeSkipped(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeReviewNudge adds the review queue to `self`, which session start runs:
// a queue nobody is told about is a queue nobody works. Silent when nothing
// awaits review; a vault that captures no sessions has no episodes folder and
// so nothing to say — unlike `arc review`, which is asked about one vault by
// name, `self` runs on every vault and must not complain about the others.
func writeReviewNudge(w io.Writer, vault string) error {
	pending, skipped, err := distill.PendingReviews(filepath.Join(vault, episodesSubdir))
	if err != nil {
		_, werr := fmt.Fprintf(w, "\nReview queue unreadable: %v\n", err)
		return werr
	}
	b := &strings.Builder{}
	if len(pending) > 0 || len(skipped) > 0 {
		b.WriteString("\n")
	}
	if len(pending) > 0 {
		fmt.Fprintf(b, "Turning points: %s (oldest %s) — nobody has judged them yet.\n  "+
			arcReviewCommand+"\n", awaitingPhrase(len(pending)), pending[0].Date, vault)
	}
	if len(skipped) > 0 {
		fmt.Fprintf(b, "%d episode(s) could not be read and are not in the queue — `arc review` names them.\n", len(skipped))
	}
	_, err = io.WriteString(w, b.String())
	return err
}
