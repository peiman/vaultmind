package memory

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// OmittedNote is a pinned note the caller can see but did not receive.
// An id alone is not enough: the session needs the title to know what to unpin,
// and the vault when more than one was read.
type OmittedNote struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Vault string `json:"vault,omitempty"`
}

// PinnedItem is one pinned note that fit. Vault is the path it was read from.
type PinnedItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	Tokens  int    `json:"tokens"`
	Vault   string `json:"vault,omitempty"`
}

// PinnedResult is every pinned note that fit, plus the names of those that did not.
type PinnedResult struct {
	Total   int           `json:"total"`
	Items   []PinnedItem  `json:"items"`
	Omitted []OmittedNote `json:"omitted,omitempty"`
	Tokens  int           `json:"tokens"`
	Budget  int           `json:"budget"`
}

// PinnedConfig bounds the pinned set. Budget 0 is unbounded. ExcerptTokens 0
// is the whole body. Both match arc recite.
type PinnedConfig struct {
	Budget        int
	ExcerptTokens int
}

// PinnedVault is one index and the vault name its notes are tagged with.
// Vaults are read in the order given. One budget covers every vault.
type PinnedVault struct {
	DB    *index.DB
	Vault string
}

// Pinned loads every note marked pinned: true in one vault, ordered by id.
//
// Drafts and gaps are included when they are pinned. Pinning is explicit.
// A note the index names and the store cannot load is named with an empty
// title, the same way recite counts a missing arc, rather than skipped.
func Pinned(db *index.DB, cfg PinnedConfig) (*PinnedResult, error) {
	return PinnedVaults([]PinnedVault{{DB: db}}, cfg)
}

// PinnedVaults loads pinned notes from each vault in order, notes by id
// within a vault, spending one budget across all of them.
func PinnedVaults(vaults []PinnedVault, cfg PinnedConfig) (*PinnedResult, error) {
	result := &PinnedResult{
		Items:  []PinnedItem{},
		Budget: cfg.Budget,
	}
	for _, v := range vaults {
		if err := addPinnedVault(result, v, cfg); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func addPinnedVault(result *PinnedResult, v PinnedVault, cfg PinnedConfig) error {
	ids, err := index.PinnedIDs(v.DB)
	if err != nil {
		return err
	}
	result.Total += len(ids)
	for _, id := range ids {
		note, err := v.DB.QueryFullNote(id)
		if err != nil {
			return fmt.Errorf("pinned: loading %q: %w", id, err)
		}
		if note == nil {
			omitPinned(result, id, "", v.Vault)
			continue
		}
		text, cost, keep := excerptForBudget(note.Body, cfg.ExcerptTokens, result.Tokens, cfg.Budget)
		if !keep {
			omitPinned(result, id, note.Title, v.Vault)
			continue
		}
		result.Items = append(result.Items, PinnedItem{
			ID: id, Title: note.Title, Excerpt: text, Tokens: cost, Vault: v.Vault,
		})
		result.Tokens += cost
	}
	return nil
}

func omitPinned(result *PinnedResult, id, title, vault string) {
	result.Omitted = append(result.Omitted, OmittedNote{ID: id, Title: title, Vault: vault})
}
