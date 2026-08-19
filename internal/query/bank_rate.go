package query

import (
	"fmt"
	"time"

	"github.com/peiman/vaultmind/internal/index"
)

// BankRate counts notes added to a vault in a window — the write side of
// memory, and the half no surface has ever reported.
//
// Retrieval has had metrics since the experiment DB existed. Banking has had
// none, and the absence shows: this vault's research half took 2 notes in 90
// days, both from one bulk import, while the sessions that should have fed it
// produced verified findings that went into commit messages instead. A number
// nobody prints is a habit nobody has.
//
// Counted from the notes table's `created` frontmatter date rather than git or
// file mtime. It is the vault's own claim about when a note came into being,
// it survives moves and re-indexing, and it is already indexed — no new state,
// no shelling out.
//
// The limit worth stating: a note back-dated by hand counts on its stated day,
// and a bulk import of 200 sources all created "today" reads as a banner day.
// The number answers "is anything being written down", not "was it earned".
type BankRate struct {
	WindowDays int
	Added      int
	// Total is every note in the vault, so a bank count reads against the size
	// of the thing it is supposedly feeding.
	Total int
}

// BankRateSince counts notes whose `created` date falls within windowDays.
func BankRateSince(db *index.DB, windowDays int) (*BankRate, error) {
	if windowDays <= 0 {
		return nil, fmt.Errorf("window must be positive, got %d", windowDays)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -windowDays).Format("2006-01-02")

	out := &BankRate{WindowDays: windowDays}
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&out.Total); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	// created is a date string ("2026-08-19"); a lexicographic >= is a date
	// comparison for ISO-8601, and only for ISO-8601 — which is why the schema
	// requires that shape rather than accepting whatever the frontmatter holds.
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM notes WHERE created IS NOT NULL AND created >= ?`, cutoff,
	).Scan(&out.Added); err != nil {
		return nil, fmt.Errorf("counting recently created notes: %w", err)
	}
	return out, nil
}
