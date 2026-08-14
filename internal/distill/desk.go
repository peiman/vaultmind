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
// There is deliberately no Distilled/DistilledTo field: ScanDesk filters
// distilled entries OUT, so such fields could only ever serialize as false/""
// and would tell a JSON consumer the opposite of something useful. Every entry
// in this list is, by construction, still pending.
type DeskEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
	Path  string `json:"path"`
	// Snippet is the head of the body, used to find similar existing arcs. It is
	// not serialized: it exists to be matched on, and echoing it back would just
	// duplicate content the reader can open by id.
	Snippet string `json:"-"`
	// NearestArcs are existing arcs this entry resembles — evidence for the
	// reader's covered/new judgement, never that judgement. See AnnotateNearestArcs.
	NearestArcs []NearArc `json:"nearest_arcs,omitempty"`
}

// ScanDesk walks deskPath and returns every journal entry NOT yet marked as
// distilled, newest first. A missing directory yields no entries and no error:
// most vaults have no desk, and arc candidates must keep working for them.
//
// Entries are not scored or ranked. The filtering that matters already happened
// when the mind chose to write one, so re-judging them here would only discard
// signal it cannot recover.
func ScanDesk(deskPath string) ([]DeskEntry, []string, error) {
	info, err := os.Stat(deskPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil, nil // no desk at all — the normal case, not a failure
	case err != nil:
		// A desk that exists but can't be read (permissions, a broken mount) is
		// NOT the same as no desk, and returning "nothing pending" for it would
		// silently report an empty backlog while entries sit unreachable.
		return nil, nil, fmt.Errorf("reading desk %q: %w", deskPath, err)
	case !info.IsDir():
		return nil, nil, nil
	}

	var entries []DeskEntry
	// A desk entry that cannot be read or parsed must be REPORTED, not dropped.
	// The report calls these "the strongest arc material there is", and this
	// module's own de-duplication design refuses a covered/new verdict precisely
	// because silently discarding a transformation is unrecoverable. Dropping one
	// over an unquoted colon in a title would do exactly that, and tell the
	// reader the desk was clear.
	var diagnostics []string
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
		entry, ok, why := readDeskEntry(path, deskPath)
		switch {
		case ok:
			entries = append(entries, entry)
		case why != "":
			rel, relErr := filepath.Rel(deskPath, path)
			if relErr != nil {
				rel = path
			}
			diagnostics = append(diagnostics, fmt.Sprintf("desk entry %s skipped: %s", rel, why))
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, fmt.Errorf("scanning desk %q: %w", deskPath, walkErr)
	}

	// Newest first: the freshest transformation is the one whose understanding
	// is still recoverable, so it is the one worth drafting soonest.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date > entries[j].Date
		}
		return entries[i].ID < entries[j].ID
	})
	sort.Strings(diagnostics)
	return entries, diagnostics, nil
}

// readDeskEntry reports whether a file is an undistilled desk entry. The third
// return is a non-empty reason when the file LOOKED like one and could not be
// used — an unreadable file or malformed frontmatter — as distinct from an
// ordinary non-entry (a README, a template), which is silent.
func readDeskEntry(path, root string) (DeskEntry, bool, string) {
	content, err := os.ReadFile(path) // #nosec G304 -- path comes from walking the caller's own vault
	if err != nil {
		return DeskEntry{}, false, "unreadable: " + err.Error()
	}
	fm, body, err := parser.ExtractFrontmatter(content)
	if err != nil {
		return DeskEntry{}, false, "malformed frontmatter: " + err.Error()
	}
	if fm == nil {
		return DeskEntry{}, false, "" // no frontmatter at all: not a note, not an error
	}
	if !strings.EqualFold(stringField(fm, "type"), deskEntryType) {
		return DeskEntry{}, false, ""
	}
	if to := stringField(fm, distilledToField); to != "" {
		// Already became an arc. Surfacing it again would train the reader to
		// skim the list, which is how a propose-only surface stops being read.
		return DeskEntry{}, false, ""
	}

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = path
	}
	return DeskEntry{
		ID:      stringField(fm, "id"),
		Title:   stringField(fm, "title"),
		Date:    stringField(fm, "date"),
		Path:    rel,
		Snippet: headOf(body, snippetMax),
	}, true, ""
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

// snippetMax bounds the body text used for arc similarity. Enough to carry the
// transformation's vocabulary, short enough that one long entry doesn't dominate
// the query.
const snippetMax = 1200

// headOf returns the first n characters of s on a rune boundary.
func headOf(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
