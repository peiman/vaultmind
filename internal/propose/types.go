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
	Sources                      []string
	JSON                         bool
	Now                          time.Time // zero means time.Now().UTC()
	ID                           string    // accept: the draft id
	Stdout, Stderr               io.Writer
	Embed                        func(vaultPath string)
	WorkDir                      string
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

// outcome is one successful propose or accept.
type outcome struct {
	Mode     string
	Path     string
	ID       string
	TargetID string
	Status   string
	Removed  string
	Warnings []string
}
