// Package gap records a question the vault did not answer.
//
// A gap is a note. The index is a regenerable cache, so a committed note is
// the only durable home (Principle 7 — one source of truth). Recall never
// returns it. A weak ask is only a hint, written to local state, and is not
// a gap.
package gap

import (
	"context"
	"io"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

const (
	statusOpen   = "open"
	statusClosed = "closed"
	modeCreated  = "created"
	modeExisting = "existing"
	commandGap   = "note gap"
	gapsDir      = "gaps"
	// typeMissing names the fix for a vault that defines no types, so gap
	// is not registered. A vault that already has types gets gap in memory.
	typeMissing = "this vault has no gap type; add types.gap to .vaultmind/config.yaml or run vaultmind init"
)

// Hits lists the note ids ask would return for a question right now.
// Drafts and gaps are already excluded by that ranking.
type Hits interface {
	IDs(ctx context.Context, question string) ([]string, error)
}

// Vault is an opened vault. The command opens it; gap does not.
type Vault struct {
	Path      string
	DB        *index.DB
	Config    *vault.Config
	Reg       *schema.Registry
	IndexHash func() string
}

// Request is one note gap call.
type Request struct {
	Question       string
	Session        string
	JSON           bool
	Now            time.Time // zero means time.Now().UTC()
	Stdout, Stderr io.Writer
}

// OpenNote is one open gap for doctor and the meter.
type OpenNote struct {
	ID       string
	Question string
	AgeDays  int
}

// Report is the gap meter. Opened counts open and closed gaps. Median is
// nil when nothing has been closed. Possible is the hint log, not a note.
type Report struct {
	Notes          []OpenNote
	Opened, Closed int
	Median         *float64
	Possible       int
	PossibleSince  string
}

// outcome is one successful note gap call.
type outcome struct {
	Mode, Path, ID, Question, Status string
	AskedHits                        []string
	Report                           Report
}
