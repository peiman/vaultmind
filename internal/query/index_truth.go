package query

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
)

// IndexTruth answers one question — does the index still describe what is on
// disk? — in every way it can be false.
//
// One reconciliation rather than one check per symptom, because the symptoms
// were three separate checks that could not fail: index_status was a string
// literal in two constructors, drift detection swallowed the read error for a
// deleted file, and duplicate_ids ran GROUP BY … HAVING COUNT(*) > 1 against a
// column declared UNIQUE. Each was a constant success signal that automation
// branched on. Fixing them apart would have produced three more checks whose
// failure modes nobody reasoned about together.
type IndexTruth struct {
	// Drifted: row and file both exist, content differs. The note was edited
	// after the last index pass, so the index, the embeddings and any rendered
	// marker sections describe an older version of it.
	Drifted []ContentDrift `json:"drifted,omitempty"`

	// Orphaned: row exists, file does not. Until the next incremental pass
	// sweeps it, the note answers queries and cannot be opened.
	Orphaned []OrphanedEntry `json:"orphaned,omitempty"`

	// Unindexed: file exists, no row. Invisible to every query, and invisible
	// to total_files too — that count comes from the database, so a
	// never-indexed note does not even show up as a mismatch.
	Unindexed []string `json:"unindexed,omitempty"`

	// Duplicate: one id claimed by two or more files on disk. Not a database
	// question — notes.id is UNIQUE, so at most one of them is ever stored and
	// the SQL check is structurally incapable of seeing the collision. Only the
	// disk can answer it.
	Duplicate []DuplicateOnDisk `json:"duplicate,omitempty"`

	// Skipped: *.md symlinks the scanner refused to follow. Reported so they
	// are distinguishable from Unindexed — a symlink is a deliberate exclusion,
	// and counting it as staleness would make every vault holding one
	// permanently unhealthy. That is how a signal gets ignored, then disabled.
	Skipped []string `json:"skipped,omitempty"`
}

// OrphanedEntry names an index row whose file is gone from disk.
type OrphanedEntry struct {
	NoteID string `json:"note_id"`
	Path   string `json:"path"`
}

// DuplicateOnDisk names one id and every file claiming it.
type DuplicateOnDisk struct {
	ID    string   `json:"id"`
	Paths []string `json:"paths"`
}

// IsCurrent reports whether the index describes the vault.
//
// Skipped is deliberately not consulted: a refused symlink is a decision, not a
// discrepancy.
func (t IndexTruth) IsCurrent() bool {
	return len(t.Drifted) == 0 &&
		len(t.Orphaned) == 0 &&
		len(t.Unindexed) == 0 &&
		len(t.Duplicate) == 0
}

// IndexStatusCurrent and IndexStatusStale are the values of the index_status
// field on doctor and vault status. Constants because two packages render them
// and consumers branch on the string.
const (
	IndexStatusCurrent = "current"
	IndexStatusStale   = "stale"

	// IndexStatusUnknown is what the field says when the reconciliation could
	// not run at all — an unreadable vault root, a config that will not load.
	//
	// A third state rather than defaulting to "current", for the reason this
	// whole file exists: "checked, and it is current" and "could not check" are
	// different facts, and collapsing them into the optimistic one is how a
	// field becomes a constant success signal. DoctorResult.ValidationSummary
	// already draws exactly this line — nil means "validation not run", &{0,0}
	// means "ran, found nothing" — and the same distinction belongs here.
	IndexStatusUnknown = "unknown"
)

// Status renders IsCurrent as the index_status field's value.
func (t IndexTruth) Status() string {
	if t.IsCurrent() {
		return IndexStatusCurrent
	}
	return IndexStatusStale
}

// diskNote is what one pass over a file yields: enough to answer drift and
// duplicate-id without reading it twice.
type diskNote struct {
	hash string
	id   string
}

// ReconcileIndex compares the index against the vault on disk.
//
// Every file is read exactly once — the content hash (drift) and the
// frontmatter id (duplicates) both come from that read.
func ReconcileIndex(db *index.DB, vaultPath string) (*IndexTruth, error) {
	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading vault config: %w", err)
	}
	scan, err := vault.Scan(vaultPath, cfg.Vault.Exclude)
	if err != nil {
		return nil, fmt.Errorf("scanning vault: %w", err)
	}

	truth := &IndexTruth{Skipped: scan.SkippedSymlinks}

	onDisk := make(map[string]diskNote, len(scan.Files))
	idPaths := make(map[string][]string)
	for _, f := range scan.Files {
		// AbsPath is produced by vault.Scan walking the vault root, and the scanner
		// refuses symlinks (vault.SkipSymlink), so it cannot name a file outside
		// the vault. Same value internal/index reads on the same guarantee.
		// nosemgrep: go-path-traversal
		content, readErr := os.ReadFile(f.AbsPath) // #nosec G304
		if readErr != nil {
			// Unreadable is not "changed" and not "missing". Skipping keeps the
			// contract the drift detector has always had: a per-file IO problem
			// must not abort a health run.
			continue
		}
		note := diskNote{hash: fmt.Sprintf("%x", sha256.Sum256(content))}
		if fm, _, parseErr := parser.ExtractFrontmatter(content); parseErr == nil {
			_, note.id, _ = parser.ClassifyNote(fm)
		}
		onDisk[f.RelPath] = note
		if note.id != "" {
			idPaths[note.id] = append(idPaths[note.id], f.RelPath)
		}
	}

	indexedPaths, err := reconcileRows(db, vaultPath, onDisk, truth)
	if err != nil {
		return nil, err
	}

	// Duplicates first: a file that lost an id contest is also, technically, a
	// file with no index row — but reporting it as "unindexed" would attach the
	// wrong remedy. `index` will skip it again for the same reason, so telling
	// the operator to re-run is advice that cannot work. The duplicate finding
	// names it and explains why it is absent; that is the actionable one.
	contested := make(map[string]bool)
	for id, paths := range idPaths {
		if len(paths) > 1 {
			sort.Strings(paths)
			truth.Duplicate = append(truth.Duplicate, DuplicateOnDisk{ID: id, Paths: paths})
			for _, p := range paths {
				contested[p] = true
			}
		}
	}
	sort.Slice(truth.Duplicate, func(i, j int) bool { return truth.Duplicate[i].ID < truth.Duplicate[j].ID })

	for _, f := range scan.Files {
		if !indexedPaths[f.RelPath] && !contested[f.RelPath] {
			truth.Unindexed = append(truth.Unindexed, f.RelPath)
		}
	}
	sort.Strings(truth.Unindexed)

	return truth, nil
}

// reconcileRows walks the index and fills Drifted and Orphaned, returning the
// set of paths the index holds so the caller can invert it into Unindexed.
func reconcileRows(db *index.DB, vaultPath string, onDisk map[string]diskNote, truth *IndexTruth) (map[string]bool, error) {
	rows, err := db.Query(`SELECT id, path, hash, is_domain FROM notes ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("querying notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	indexed := make(map[string]bool)
	for rows.Next() {
		var id, path, storedHash string
		var isDomain bool
		if scanErr := rows.Scan(&id, &path, &storedHash, &isDomain); scanErr != nil {
			return nil, fmt.Errorf("scanning note row: %w", scanErr)
		}
		indexed[path] = true

		note, present := onDisk[path]
		if !present {
			// Not in the scan: either gone, or excluded by config, or unreadable.
			// Only a missing FILE is an orphan — asking the filesystem keeps a
			// config change from reading as a vault full of deleted notes.
			if _, statErr := os.Stat(filepath.Join(vaultPath, path)); os.IsNotExist(statErr) {
				truth.Orphaned = append(truth.Orphaned, OrphanedEntry{NoteID: id, Path: path})
			}
			continue
		}
		// Drift stays domain-only: an unstructured note has no schema contract
		// and its edits do not invalidate downstream artifacts the same way.
		if isDomain && note.hash != storedHash {
			truth.Drifted = append(truth.Drifted, ContentDrift{
				NoteID:      id,
				Path:        path,
				CurrentHash: note.hash,
				StoredHash:  storedHash,
			})
		}
	}
	return indexed, rows.Err()
}
