package cmd

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/gap"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
)

// attachGapReview fills the gap meter. A failure is reported as an empty
// measured list, not by failing doctor.
func attachGapReview(result *query.DoctorResult, vdb *cmdutil.VaultDB, vaultPath string) {
	report, err := gap.Measure(vdb.DB, vaultPath, time.Now())
	if err != nil {
		log.Debug().Err(err).Msg("gap list failed")
		result.Gaps = &query.DoctorGaps{Notes: []query.DoctorGapNote{}}
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
