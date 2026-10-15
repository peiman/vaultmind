package cmd

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/gap"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
)

// attachGapReview fills the gap meter. A failure to list gaps is reported as
// an empty measured list, not by failing doctor. The cite count is copied
// from memory use and stays null when that section was not measured.
func attachGapReview(result *query.DoctorResult, vdb *cmdutil.VaultDB, vaultPath string) {
	cited := citedFromMemory(result.MemoryUse)
	report, err := gap.Measure(vdb.DB, vaultPath, time.Now())
	if err != nil {
		log.Debug().Err(err).Msg("gap list failed")
		result.Gaps = &query.DoctorGaps{Notes: []query.DoctorGapNote{}}
		result.GapLoop = &query.DoctorGapLoop{Scope: query.UsageScopeMachine, GapClosingCited: cited}
		return
	}
	notes := make([]query.DoctorGapNote, 0, len(report.Notes))
	for _, n := range report.Notes {
		notes = append(notes, query.DoctorGapNote{ID: n.ID, Question: n.Question, AgeDays: n.AgeDays})
	}
	result.Gaps = &query.DoctorGaps{
		Notes:             notes,
		Opened:            report.Opened,
		Closed:            report.Closed,
		MedianDaysToClose: report.Median,
		Possible:          report.Possible,
		PossibleSince:     report.PossibleSince,
	}
	result.GapLoop = &query.DoctorGapLoop{
		Scope:             query.UsageScopeMachine,
		Opened:            report.Opened,
		Closed:            report.Closed,
		MedianDaysToClose: report.Median,
		GapClosingCited:   cited,
	}
}

// citedFromMemory copies a measured cite count. No usage log is unmeasured:
// a zero there would be a guess (Principle 1).
func citedFromMemory(use *query.DoctorMemoryUse) *int {
	if use == nil {
		return nil // no log: unmeasured, not zero
	}
	return use.GapClosingCited
}

// gapClosingCounts is the cite half of the claim. The id set is the same
// lookup ask uses. A failed lookup or cite query is unmeasured, not zero,
// and does not fail doctor.
func gapClosingCounts(vault *index.DB, exp *experiment.DB) (surfaced, cited *int) {
	if vault == nil || exp == nil {
		return nil, nil
	}
	credits, err := index.GapClosures(vault)
	if err != nil {
		log.Debug().Err(err).Msg("gap credit lookup failed")
		return nil, nil
	}
	ids := make([]string, 0, len(credits))
	for id := range credits {
		ids = append(ids, id)
	}
	nSurfaced, nCited, err := exp.GapClosingUse(memoryUseWindowDays, ids)
	if err != nil {
		log.Debug().Err(err).Msg("gap-closing use failed")
		return nil, nil
	}
	return countPtr(nSurfaced), countPtr(nCited)
}

func countPtr(n int) *int {
	v := n
	return &v
}

func gapCountText(n *int) string {
	if n == nil {
		return "n/a"
	}
	return strconv.Itoa(*n)
}

func writeGapReview(w io.Writer, g *query.DoctorGaps) error {
	if g == nil {
		return nil
	}
	open := len(g.Notes)
	if _, err := fmt.Fprintf(w, "Open gaps: %d\n", open); err != nil {
		return err
	}
	for _, n := range g.Notes {
		if _, err := fmt.Fprintf(w, "  %s  %s  %dd\n", n.ID, n.Question, n.AgeDays); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Gaps opened: %d\nGaps closed: %d\nMedian days to close: %s\n",
		g.Opened, g.Closed, gapMedianText(g.MedianDaysToClose)); err != nil {
		return err
	}
	if g.Possible == 0 {
		_, err := fmt.Fprintln(w, "0 weak asks")
		return err
	}
	_, err := fmt.Fprintf(w, "%d weak asks since %s\n", g.Possible, g.PossibleSince)
	return err
}

func gapMedianText(m *float64) string {
	if m == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*m, 'f', -1, 64)
}
