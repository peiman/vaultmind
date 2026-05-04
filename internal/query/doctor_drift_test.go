package query_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind doctor content-drift detection. Tests pin the contract:
// when a note's current file content hash differs from the indexer's
// stored hash, doctor reports it as "stale index" — i.e., the index
// is out of sync with the file. Detection is content-hash based, NOT
// mtime-based: git checkouts, branch switches, and other VCS
// operations bump file mtime without touching content, and the prior
// mtime-based detector produced ~95% false positives in real vaults
// (385 of 407 notes). Hash comparison is precise: only actual content
// edits trigger drift.

func openDriftTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(t.TempDir() + "/drift-test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sha256Hex(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h[:])
}

func writeNoteFileForDrift(t *testing.T, vault, name, body string) string {
	t.Helper()
	full := filepath.Join(vault, name)
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	return full
}

// TestDetectContentDrift_MatchingHashNotFlagged — current file hash
// matches stored DB hash → no drift.
func TestDetectContentDrift_MatchingHashNotFlagged(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	body := `---
id: ref-clean
type: reference
title: Clean
---
body
`
	writeNoteFileForDrift(t, vault, "clean.md", body)
	_, err := db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ref-clean", "clean.md", "reference", "Clean",
		sha256Hex([]byte(body)), 0, true)
	require.NoError(t, err)

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err)
	assert.Empty(t, drifts, "matching hash means no drift")
}

// TestDetectContentDrift_HashMismatchFlagged — current file hash
// differs from stored DB hash → drift reported with both hashes.
func TestDetectContentDrift_HashMismatchFlagged(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	currentBody := `---
id: ref-edited
type: reference
title: Edited
---
NEW BODY
`
	writeNoteFileForDrift(t, vault, "edited.md", currentBody)
	// DB stores a stale hash from a hypothetical earlier indexing.
	staleHash := sha256Hex([]byte("OLD BODY"))
	_, err := db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ref-edited", "edited.md", "reference", "Edited",
		staleHash, 0, true)
	require.NoError(t, err)

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err)
	require.Len(t, drifts, 1)
	assert.Equal(t, "edited.md", drifts[0].Path)
	assert.Equal(t, "ref-edited", drifts[0].NoteID)
	assert.Equal(t, sha256Hex([]byte(currentBody)), drifts[0].CurrentHash)
	assert.Equal(t, staleHash, drifts[0].StoredHash)
}

// TestDetectContentDrift_MtimeChangeAloneNoFlag — the false-positive
// fix that justified this rewrite. A note's file mtime bumps (as
// happens during git checkouts / branch switches / pulls) but
// content is byte-identical. Hash matches; no drift. Without this
// behavior, real vaults reported 385 of 407 notes as drift after
// nothing more than `git checkout main`.
func TestDetectContentDrift_MtimeChangeAloneNoFlag(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	body := `---
id: ref-checkout
type: reference
title: Checkout
---
unchanged body
`
	notePath := writeNoteFileForDrift(t, vault, "checkout.md", body)
	hash := sha256Hex([]byte(body))
	_, err := db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ref-checkout", "checkout.md", "reference", "Checkout",
		hash, 100, true)
	require.NoError(t, err)

	// Simulate the "git checkout" effect: mtime changes, content does not.
	// We rewrite the same content (no-op for hash) and let the OS bump mtime.
	require.NoError(t, os.WriteFile(notePath, []byte(body), 0o644))

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err)
	assert.Empty(t, drifts,
		"mtime bump without content change must NOT be flagged — this is the whole reason for hash-based detection")
}

// TestDetectContentDrift_DeletedFileSilent — file removed from disk
// after indexing. Stat fails; the note is silently skipped (the
// indexer reports missing-file errors via its own path; doctor's job
// is health summary, not file-error reporting).
func TestDetectContentDrift_DeletedFileSilent(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	_, err := db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ref-gone", "gone.md", "reference", "Gone",
		"deadbeef", 0, true)
	require.NoError(t, err)

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err, "missing-file errors must not abort the run")
	assert.Empty(t, drifts, "deleted files are silently skipped")
}

// TestDetectContentDrift_NonDomainNotIncluded — non-domain notes
// (READMEs, drafts, anything is_domain=FALSE) are skipped. Drift is
// a signal about indexed semantic content, not arbitrary markdown.
func TestDetectContentDrift_NonDomainNotIncluded(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	body := `# README\n\njust content\n`
	writeNoteFileForDrift(t, vault, "README.md", body)
	// Non-domain: is_domain = false. Stored hash is intentionally
	// stale; if we wrongly included non-domain notes, this would surface
	// as drift.
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?)`,
		"_path:README.md", "README.md", "stale-hash", 0, false)
	require.NoError(t, err)

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err)
	assert.Empty(t, drifts, "non-domain notes are excluded from drift detection")
}

// TestDetectContentDrift_MultipleNotesMixed — across a vault with
// clean, drifted, and missing notes, only the actual content
// mismatches are flagged. Pins the population semantics under realistic
// conditions.
func TestDetectContentDrift_MultipleNotesMixed(t *testing.T) {
	vault := t.TempDir()
	db := openDriftTestDB(t)

	cleanBody := "---\nid: a\ntype: reference\n---\nclean\n"
	driftedBody := "---\nid: b\ntype: reference\n---\nedited content\n"
	writeNoteFileForDrift(t, vault, "a.md", cleanBody)
	writeNoteFileForDrift(t, vault, "b.md", driftedBody)

	_, err := db.Exec(`INSERT INTO notes (id, path, type, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
		"a", "a.md", "reference", sha256Hex([]byte(cleanBody)), 0, true)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, type, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
		"b", "b.md", "reference", "stale-hash", 0, true)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, type, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
		"c", "c-missing.md", "reference", "any", 0, true)
	require.NoError(t, err)

	drifts, err := query.DetectContentDrift(db, vault)
	require.NoError(t, err)
	require.Len(t, drifts, 1, "only the drifted note appears")
	assert.Equal(t, "b.md", drifts[0].Path)
}
