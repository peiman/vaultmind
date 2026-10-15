package query

import (
	"fmt"
	"io"
	"time"

	"github.com/peiman/vaultmind/internal/index"
)

// gapCreditQuestionRunes is how much of the question the delivery line quotes.
// The JSON field keeps the whole question.
const gapCreditQuestionRunes = 80

// loadGapClosures is the one lookup per command. Tests replace it with a
// counting stub. Production is index.GapClosures.
var loadGapClosures = index.GapClosures

// gapCreditZone is the clock for the date on the credit line. JSON keeps
// opened_at in UTC. Tests set a fixed zone; production is the local zone.
var gapCreditZone = time.Local

// attachGapCredits stamps a delivered note with the gap it closed, and only
// where the text line would be: the target, or a context item whose body was
// delivered, and not a note already shown. A hit gets the field only when
// that same note does. A draft or a gap never carries it. One lookup covers
// the command. A nil db is an empty map: there is nothing to credit.
func attachGapCredits(db *index.DB, result *AskResult) error {
	if result == nil {
		return nil
	}
	credits, err := loadGapClosures(gapCreditQuery(db))
	if err != nil {
		return err
	}
	if len(credits) == 0 {
		return nil
	}
	hidden, err := index.DraftIDs(gapCreditQuery(db))
	if err != nil {
		return err
	}
	credited := map[string]*index.GapCredit{}
	if result.Context != nil {
		if target := result.Context.Target; target != nil {
			id := target.ID
			if id == "" {
				id = result.Context.TargetID
			}
			target.AnswersGap = lineCredit(credits, hidden, id, target.ShownBefore, target.Body != "")
			if target.AnswersGap != nil {
				credited[id] = target.AnswersGap
			}
		}
		for i := range result.Context.Context {
			item := &result.Context.Context[i]
			item.AnswersGap = lineCredit(credits, hidden, item.ID, item.ShownBefore, item.BodyIncluded && item.Body != "")
			if item.AnswersGap != nil {
				credited[item.ID] = item.AnswersGap
			}
		}
	}
	for i := range result.TopHits {
		result.TopHits[i].AnswersGap = credited[result.TopHits[i].ID]
	}
	return nil
}

// lineCredit returns the credit only when the text line would name this note.
func lineCredit(credits map[string]index.GapCredit, hidden index.DraftSet, id string, shownBefore, delivered bool) *index.GapCredit {
	if shownBefore || !delivered || hidden.Contains(id) {
		return nil
	}
	return gapCreditFor(credits, id)
}

// gapCreditQuery passes a nil interface for a nil *DB. A nil pointer stored
// in a RowQuery is not a nil interface, and Query on it would panic.
func gapCreditQuery(db *index.DB) index.RowQuery {
	if db == nil {
		return nil
	}
	return db
}

func gapCreditFor(credits map[string]index.GapCredit, id string) *index.GapCredit {
	credit, ok := credits[id]
	if !ok {
		return nil
	}
	copied := credit
	return &copied
}

// writeGapCreditLine is the one line under a gap-closing note whose body
// was delivered. Pointers-only and a note already shown this session stay
// quiet: the line is evidence the agent can read, not a repeated banner.
// A draft or a gap is not knowledge, so it never carries the line.
func writeGapCreditLine(w io.Writer, credit *index.GapCredit, shownBefore, delivered, knowledge bool) error {
	if credit == nil || shownBefore || !delivered || !knowledge {
		return nil
	}
	question := credit.Question
	if chars := []rune(question); len(chars) > gapCreditQuestionRunes {
		question = string(chars[:gapCreditQuestionRunes])
	}
	_, err := fmt.Fprintf(w, "  ↳ answers a question first investigated %s: %q\n",
		credit.OpenedAt.In(gapCreditZone).Format(time.DateOnly), question)
	return err
}

// knowledgeNote is false for a draft or a gap. Empty frontmatter is knowledge:
// the non-knowledge set is what attach already applied.
func knowledgeNote(frontmatter map[string]any) bool {
	if noteType, _ := frontmatter["type"].(string); noteType == index.TypeGap {
		return false
	}
	if status, _ := frontmatter["status"].(string); status == index.StatusDraft {
		return false
	}
	return true
}
