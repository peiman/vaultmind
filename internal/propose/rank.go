package propose

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/vault"
	"gopkg.in/yaml.v3"
)

// measuredOnProposal is the rank_check.measured_on value for a patch.
// The finding lives on the proposal note, so the after rank is that note's.
const measuredOnProposal = "proposal"

// closeRanker releases a ranker that holds a model. A ranker that does not
// is left alone. Closing before the model loaded is a no-op the ranker owns.
func closeRanker(req Request) {
	if req.Ranker == nil {
		return
	}
	c, ok := req.Ranker.(io.Closer)
	if !ok {
		return
	}
	_ = c.Close()
}

// attachRankCheck measures whether the draft repairs retrieval of the
// original question. The two passes differ only by whether drafts count.
// Ranking stays the ranker's, which the command implements with query.Rank,
// the same function ask uses (Principle 8). The three frontmatter keys go
// through mutation, the one writer (Principle 7). A failure leaves the draft
// on disk and names it: a retry would see the existing draft and not check again.
func attachRankCheck(ctx context.Context, v Vault, req Request, out *outcome, now time.Time) error {
	if req.Ranker == nil || out == nil {
		return nil
	}
	excluded, err := req.Ranker.RankQuestion(ctx, req.Question, false)
	if err != nil {
		return rankCheckErr(req, out.ID, err)
	}
	included, err := req.Ranker.RankQuestion(ctx, req.Question, true)
	if err != nil {
		return rankCheckErr(req, out.ID, err)
	}
	check := measureRank(out.Mode, out.ID, out.TargetID, excluded, included)
	// The frontmatter write changes the note hash, which drops its vectors,
	// and the re-index embeds them again. That embed borrows the process
	// session, so this close only drops the ranker's own cleanup. It must
	// not unload the session. Run closes the ranker again on return; Close
	// is idempotent.
	closeRanker(req)
	warnings, err := writeRankKeys(ctx, v, req, out.Path, now, check)
	if err != nil {
		return rankCheckErr(req, out.ID, err)
	}
	out.Warnings = append(out.Warnings, warnings...)
	out.Rank = &check
	return nil
}

func rankCheckErr(req Request, id string, err error) error {
	return failOp(req, cmdPropose, "retrieval_failed", fmt.Errorf("retrieval check for %s: %w", id, err))
}

// measureRank reads the two windows. before is the patch target, or for a
// new note the best hit that is not a draft. after is the draft itself.
// A miss is rank 0. The outranked ids are the window that beat a miss.
func measureRank(mode, draftID, targetID string, excluded, included QuestionRank) RankCheck {
	modeName, window := included.Mode, included.Window
	if modeName == "" {
		modeName = excluded.Mode
	}
	if window == 0 {
		window = excluded.Window
	}
	subject := targetID
	if mode != modePatch {
		subject = ""
		if len(excluded.Hits) > 0 {
			subject = excluded.Hits[0].ID
		}
	}
	beforeRank, beforeBand := rankOf(excluded.Hits, subject)
	afterRank, afterBand := rankOf(included.Hits, draftID)
	check := RankCheck{
		Mode:   modeName,
		Window: window,
		Before: RankPlace{ID: subject, Rank: beforeRank, Band: beforeBand},
		After:  RankPlace{ID: draftID, Rank: afterRank, Band: afterBand},
	}
	if mode == modePatch {
		check.MeasuredOn = measuredOnProposal
	}
	if afterRank == 0 {
		for _, hit := range included.Hits {
			if hit.ID == "" || hit.ID == draftID {
				continue
			}
			check.OutrankedBy = append(check.OutrankedBy, hit.ID)
		}
	}
	return check
}

func rankOf(hits []RankedHit, id string) (int, string) {
	if id == "" {
		return 0, ""
	}
	for i, hit := range hits {
		if hit.ID == id {
			return i + 1, hit.Band
		}
	}
	return 0, ""
}

// writeRankKeys stores the measurement on the draft. 0 is written for a miss
// so a reviewer can tell "checked, not in the window" from "never checked".
func writeRankKeys(ctx context.Context, v Vault, req Request, rel string, now time.Time, check RankCheck) ([]string, error) {
	abs, err := vault.ResolveInside(v.Path, rel)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(abs) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
	if err != nil {
		return nil, fmt.Errorf("reading note %s: %w", rel, err)
	}
	checkedAt := now.UTC().Format(time.RFC3339)
	err = rewriteNote(abs, rel, raw, nil, func(mapping *yaml.Node) error {
		if err := mutation.SetKey(mapping, "rank_before", check.Before.Rank); err != nil {
			return err
		}
		if err := mutation.SetKey(mapping, "rank_after", check.After.Rank); err != nil {
			return err
		}
		return mutation.SetKey(mapping, "rank_checked_at", checkedAt)
	}, nil)
	if err != nil {
		return nil, err
	}
	return reindex(ctx, v, req, rel), nil
}

// formatRankCheck is the one text line. An empty band omits its parentheses.
// An empty mode omits its parentheses. A patch says the measurement is the
// proposal note, because that note holds the finding.
func formatRankCheck(check RankCheck) string {
	var b strings.Builder
	b.WriteString("retrieval check")
	if check.Mode != "" {
		fmt.Fprintf(&b, " (%s)", check.Mode)
	}
	b.WriteString(": before — ")
	b.WriteString(formatBefore(check))
	b.WriteString("; after — ")
	b.WriteString(formatAfter(check))
	if check.MeasuredOn == measuredOnProposal {
		b.WriteString(". Measured on the proposal note, which holds the finding.")
	}
	return b.String()
}

func formatBefore(check RankCheck) string {
	if check.Before.ID == "" {
		return fmt.Sprintf("no existing note in the top %d", check.Window)
	}
	if check.Before.Rank == 0 {
		return fmt.Sprintf("best answer %s is not in the top %d", check.Before.ID, check.Window)
	}
	return fmt.Sprintf("best answer %s at #%d%s", check.Before.ID, check.Before.Rank, bandSuffix(check.Before.Band))
}

func formatAfter(check RankCheck) string {
	if check.After.Rank == 0 {
		msg := fmt.Sprintf("this draft does not rank in the top %d for its own question; consider adding the question's key terms", check.Window)
		if len(check.OutrankedBy) > 0 {
			msg += " or links to " + strings.Join(check.OutrankedBy, ", ")
		}
		return msg
	}
	return fmt.Sprintf("this draft at #%d%s", check.After.Rank, bandSuffix(check.After.Band))
}

func bandSuffix(band string) string {
	if band == "" {
		return ""
	}
	return " (" + band + ")"
}
