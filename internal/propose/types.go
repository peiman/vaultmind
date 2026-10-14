// Package propose turns a finding into a reviewable draft, and accepts that draft.
//
// The agent supplies the question and the finding. VaultMind chooses the note,
// the id, and the formatting. A person promotes the draft with note accept.
package propose

import (
	"context"
	"io"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// Hit is the closest note ask would have ranked first, plus the relevance
// band ask already computed for it. Band is empty when that judgement is
// withheld or there is no band.
type Hit struct {
	ID, Path, Type, Title, Band string
}

// Retriever finds the closest note. Drafts are already excluded by the
// retrieval the caller shares with ask.
type Retriever interface {
	Closest(ctx context.Context, vaultPath string, db *index.DB, text string) (Hit, bool, error)
}

// Vault is an opened vault. Propose does not open one: the command layer does,
// so a guessed path cannot create an index here.
type Vault struct {
	Path      string
	DB        *index.DB
	Config    *vault.Config
	Reg       *schema.Registry
	IndexHash func() string
}

// Request is one propose or accept call.
type Request struct {
	Question, Finding, CloseBand string
	// GapID, when set, stores closes_gap on the draft. An empty question
	// uses the gap's stored question.
	GapID          string
	Sources        []string
	JSON           bool
	Now            time.Time // zero means time.Now().UTC()
	ID             string    // accept: the draft id
	Stdout, Stderr io.Writer
	Embed          func(vaultPath string)
	WorkDir        string
	// Ranker re-runs the original question after the draft is on disk.
	// Nil skips the check. The command sets it; tests that only place a
	// draft leave it nil so their output stays the placement report.
	Ranker QuestionRanker
	// beforeWrite, when set, runs after the note is read and before the
	// conflict-checked write. Tests use it to change the file in that window.
	beforeWrite func(abs string) error
	// deleteIndexed removes the proposal's index row. Nil uses
	// index.DeleteNoteByPath. Tests set it to fail one delete.
	deleteIndexed func(db *index.DB, rel string) error
	// removeFile removes the proposal file. Nil uses os.Remove. Tests set
	// it to fail one delete.
	removeFile func(path string) error
}

const (
	cmdPropose = "note propose"
	cmdAccept  = "note accept"

	modePatch    = "patch"
	modeNew      = "new"
	modeExisting = "existing"
)

// RankedHit is one note in a ranking window, rank 1 first. Band is empty
// when ask would not show one for that hit.
type RankedHit struct {
	ID, Band string
}

// QuestionRank is one pass of the question through the ranking ask uses.
// Mode is "hybrid" or "keyword". Window is the hit limit of that pass.
// Zero means the caller asked for no cut.
type QuestionRank struct {
	Hits   []RankedHit
	Mode   string
	Window int
}

// QuestionRanker ranks a question. includeDrafts is the only difference
// between the two passes: false is what an agent gets today, true is the
// draft counted as if it had been accepted.
type QuestionRanker interface {
	RankQuestion(ctx context.Context, question string, includeDrafts bool) (QuestionRank, error)
}

// RankPlace is one side of the retrieval check. Rank 0 means the note was
// not inside the window.
type RankPlace struct {
	ID   string `json:"id,omitempty"`
	Rank int    `json:"rank"`
	Band string `json:"band,omitempty"`
}

// RankCheck is the before/after measurement stored on the draft and
// reported to the caller. MeasuredOn is "proposal" when the finding lives
// on the proposal note rather than on the note it patches.
type RankCheck struct {
	Mode        string    `json:"mode,omitempty"`
	Window      int       `json:"window"`
	Before      RankPlace `json:"before"`
	After       RankPlace `json:"after"`
	MeasuredOn  string    `json:"measured_on,omitempty"`
	OutrankedBy []string  `json:"outranked_by,omitempty"`
	// Decision is the hit that chose patch or new: the closest note on the
	// question and the finding. Rank is 1 when that note was found. The zero
	// value means the retrieval named nothing. Before stays the question-only
	// rank stored as rank_before.
	Decision RankPlace `json:"decision,omitempty"`
	// DecisionOn is the text that comparison used.
	DecisionOn string `json:"decision_on,omitempty"`
	// CloseBand is the weakest band the decision treated as close enough.
	CloseBand string `json:"close_band,omitempty"`
}

// outcome is one successful propose or accept.
type outcome struct {
	Mode     string
	Path     string
	ID       string
	TargetID string
	Status   string
	Removed  string
	Warnings []string
	// MissingGap is a closes_gap id that is not in the vault. Accept still
	// publishes the note. The warning names this id. An empty string means
	// there was nothing to warn about.
	MissingGap string
	Rank       *RankCheck
}
