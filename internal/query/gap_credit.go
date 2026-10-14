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

// attachGapCredits stamps every delivered hit with the gap it closed.
// One lookup covers the target, the context items, and the hit list.
// A nil db is an empty map: there is nothing to credit.
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
	for i := range result.TopHits {
		result.TopHits[i].AnswersGap = gapCreditFor(credits, result.TopHits[i].ID)
	}
	if result.Context == nil {
		return nil
	}
	if target := result.Context.Target; target != nil {
		id := target.ID
		if id == "" {
			id = result.Context.TargetID
		}
		target.AnswersGap = gapCreditFor(credits, id)
	}
	for i := range result.Context.Context {
		item := &result.Context.Context[i]
		item.AnswersGap = gapCreditFor(credits, item.ID)
	}
	return nil
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
// JSON still carries answers_gap on the note.
func writeGapCreditLine(w io.Writer, credit *index.GapCredit, shownBefore, delivered bool) error {
	if credit == nil || shownBefore || !delivered {
		return nil
	}
	question := credit.Question
	if chars := []rune(question); len(chars) > gapCreditQuestionRunes {
		question = string(chars[:gapCreditQuestionRunes])
	}
	_, err := fmt.Fprintf(w, "  ↳ answers a question first investigated %s: %q\n",
		credit.OpenedAt.UTC().Format(time.DateOnly), question)
	return err
}
