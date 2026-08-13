package distill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/parser"
)

// deskEntryType is the note type that marks raw, self-recorded transformation
// material — the desk. Journal entries are written by the mind mid-session,
// deliberately and without a curation gate, which is what makes them the
// highest-signal arc source available.
const deskEntryType = "journal"

// distilledToField, when present on a desk entry, names the arc that entry
// became. It lives on the NOTE rather than in a link the graph maintains,
// because the desk and the identity vault are separate vaults: a cross-vault
// edge would need infrastructure that does not exist, while a frontmatter field
// travels with the file and survives re-indexing, moves, and copies.
const distilledToField = "distilled_to"

// DeskEntry is a raw transformation record awaiting distillation into an arc.
//
// It is deliberately NOT a Candidate. A Candidate is a guess — a phrase matched,
// go look. A desk entry is a judgement the mind already made: it stopped
// mid-session and decided this was worth keeping. Collapsing the two would
// force a fake episode ID and turn index onto something that has neither, and
// would flatten a real difference in how much the reader should trust the item.
type DeskEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	Path        string `json:"path"`
	Distilled   bool   `json:"distilled"`
	DistilledTo string `json:"distilled_to,omitempty"`
}

// ScanDesk walks deskPath and returns every journal entry NOT yet marked as
// distilled, newest first. A missing directory yields no entries and no error:
// most vaults have no desk, and arc candidates must keep working for them.
//
// Entries are not scored or ranked. The filtering that matters already happened
// when the mind chose to write one, so re-judging them here would only discard
// signal it cannot recover.
func ScanDesk(deskPath string) ([]DeskEntry, error) {
	info, err := os.Stat(deskPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil // no desk at all — the normal case, not a failure
	case err != nil:
		// A desk that exists but can't be read (permissions, a broken mount) is
		// NOT the same as no desk, and returning "nothing pending" for it would
		// silently report an empty backlog while entries sit unreachable.
		return nil, fmt.Errorf("reading desk %q: %w", deskPath, err)
	case !info.IsDir():
		return nil, nil
	}

	var entries []DeskEntry
	walkErr := filepath.WalkDir(deskPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the vault's own machinery (.vaultmind/, .git/, .obsidian/).
			if strings.HasPrefix(d.Name(), ".") && path != deskPath {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		if entry, ok := readDeskEntry(path, deskPath); ok {
			entries = append(entries, entry)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scanning desk %q: %w", deskPath, walkErr)
	}

	// Newest first: the freshest transformation is the one whose understanding
	// is still recoverable, so it is the one worth drafting soonest.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date > entries[j].Date
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

// readDeskEntry reports whether a file is an undistilled desk entry, and its
// details if so. It returns no error by design: "unreadable" and "no
// frontmatter" are not failures here, they are simply answers of "not a desk
// entry". A vault legitimately holds READMEs, drafts, and templates, and
// aborting the scan over one of them would make the feature useless in any real
// directory. Failures that DO matter — an unreadable desk root, a broken walk —
// are caught by the caller, which can distinguish them from ordinary files.
func readDeskEntry(path, root string) (DeskEntry, bool) {
	content, err := os.ReadFile(path) // #nosec G304 -- path comes from walking the caller's own vault
	if err != nil {
		return DeskEntry{}, false
	}
	fm, _, err := parser.ExtractFrontmatter(content)
	if err != nil || fm == nil {
		return DeskEntry{}, false
	}
	if !strings.EqualFold(stringField(fm, "type"), deskEntryType) {
		return DeskEntry{}, false
	}
	if to := stringField(fm, distilledToField); to != "" {
		// Already became an arc. Surfacing it again would train the reader to
		// skim the list, which is how a propose-only surface stops being read.
		return DeskEntry{}, false
	}

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = path
	}
	return DeskEntry{
		ID:    stringField(fm, "id"),
		Title: stringField(fm, "title"),
		Date:  stringField(fm, "date"),
		Path:  rel,
	}, true
}

// stringField reads a frontmatter value as a string.
//
// An unquoted `date: 2026-08-13` is decoded by YAML as a time.Time, not a
// string, so the generic fallback would render it "2026-08-13 00:00:00 +0000
// UTC" — which then sorts and displays wrong. Dates are normalized back to the
// date-only form the file actually contains. Anything else non-empty is
// formatted rather than dropped, so an unexpected type still surfaces.
func stringField(fm map[string]interface{}, key string) string {
	v, ok := fm[key]
	if !ok || v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		return typed.Format(dateLayout)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

// dateLayout is the date-only form desk filenames and frontmatter both use.
const dateLayout = "2006-01-02"
