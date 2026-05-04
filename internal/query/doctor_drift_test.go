package query_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind doctor drift detection (commit 5 of the schema foundation
// chain). Tests pin the contract: when a note's file mtime is later
// than its vm_updated frontmatter timestamp, doctor counts that note
// as "edited since vaultmind processed it" — the operator-visible
// stale-vs-vaultmind health signal. The drift detector reads the
// filesystem (not the index DB) so the comparison reflects current
// reality, not last-index-pass state.

// touchFile sets a file's mtime to the given time. Returns an error
// from t.Helper for fail-fast; t.Helper attribution makes the assertion
// site land at the caller, not here.
func touchFile(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

// writeNoteFile writes a file at path under vault with the given body
// (frontmatter + content). Returns the absolute path.
func writeNoteFile(t *testing.T, vault, name, body string) string {
	t.Helper()
	full := filepath.Join(vault, name)
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	return full
}

// TestDetectVMUpdatedDrift_StaleNoteFlagged — note's file mtime is
// hours after its vm_updated → counted as drift.
func TestDetectVMUpdatedDrift_StaleNoteFlagged(t *testing.T) {
	vault := t.TempDir()
	notePath := writeNoteFile(t, vault, "stale.md", `---
id: ref-stale
type: reference
title: Stale
vm_updated: "2026-01-01T00:00:00Z"
---
body
`)
	// File mtime ~ 6 hours later than vm_updated → unambiguous drift.
	touchFile(t, notePath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"stale.md"})
	require.NoError(t, err)
	require.Len(t, drifts, 1, "stale note must be reported")
	assert.Equal(t, "stale.md", drifts[0].Path)
	assert.Equal(t, "ref-stale", drifts[0].NoteID)
	assert.Equal(t, "2026-01-01T00:00:00Z", drifts[0].VMUpdated)
	// Mtime is reported in canonical RFC3339-second form.
	assert.Equal(t, "2026-01-01T06:00:00Z", drifts[0].Mtime)
}

// TestDetectVMUpdatedDrift_FreshNoteNotFlagged — note whose mtime
// matches vm_updated within tolerance is NOT flagged. This avoids
// false-positive drift on vaultmind's own recent writes (the file
// write completes a fraction of a second after vm_updated is
// computed, so mtime is naturally epsilon-ahead).
func TestDetectVMUpdatedDrift_FreshNoteNotFlagged(t *testing.T) {
	vault := t.TempDir()
	notePath := writeNoteFile(t, vault, "fresh.md", `---
id: ref-fresh
type: reference
title: Fresh
vm_updated: "2026-01-01T00:00:00Z"
---
body
`)
	// File mtime 1 second after vm_updated — within the 5s tolerance.
	touchFile(t, notePath, time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"fresh.md"})
	require.NoError(t, err)
	assert.Empty(t, drifts, "same-second mtime is not drift")
}

// TestDetectVMUpdatedDrift_MissingVMUpdatedNotFlagged — a note WITHOUT
// vm_updated is NOT counted as drift; that's frontmatter fix's
// territory. Drift specifically means present-but-stale, the signal
// that "this file changed AFTER vaultmind last touched it." Absent
// vm_updated is the signal that "vaultmind never touched this file."
// Conflating the two would double-count and confuse the operator.
func TestDetectVMUpdatedDrift_MissingVMUpdatedNotFlagged(t *testing.T) {
	vault := t.TempDir()
	notePath := writeNoteFile(t, vault, "no-vmu.md", `---
id: ref-no-vmu
type: reference
title: NoVMU
---
body
`)
	touchFile(t, notePath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"no-vmu.md"})
	require.NoError(t, err)
	assert.Empty(t, drifts, "missing vm_updated is fix's signal, not drift's")
}

// TestDetectVMUpdatedDrift_UnparseableVMUpdatedFlagged — a note with
// vm_updated set but in a format vaultmind can't parse counts as
// drift: vaultmind wrote in a format it understands, so corruption
// means the value can't be trusted as a "vaultmind-touched" claim.
// Surface it so the operator can run `frontmatter fix` to reset it.
func TestDetectVMUpdatedDrift_UnparseableVMUpdatedFlagged(t *testing.T) {
	vault := t.TempDir()
	notePath := writeNoteFile(t, vault, "corrupt-vmu.md", `---
id: ref-corrupt
type: reference
title: Corrupt
vm_updated: "not a date"
---
body
`)
	touchFile(t, notePath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"corrupt-vmu.md"})
	require.NoError(t, err)
	require.Len(t, drifts, 1, "unparseable vm_updated is drift")
	assert.Equal(t, "not a date", drifts[0].VMUpdated)
}

// TestDetectVMUpdatedDrift_MultipleNotes — mixed fresh/stale/missing
// vault. Only the stale note appears; others are correctly excluded.
func TestDetectVMUpdatedDrift_MultipleNotes(t *testing.T) {
	vault := t.TempDir()
	// Stale.
	stalePath := writeNoteFile(t, vault, "stale.md", `---
id: a
type: reference
vm_updated: "2026-01-01T00:00:00Z"
---
`)
	touchFile(t, stalePath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))
	// Fresh.
	freshPath := writeNoteFile(t, vault, "fresh.md", `---
id: b
type: reference
vm_updated: "2026-01-01T00:00:00Z"
---
`)
	touchFile(t, freshPath, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// Missing vm_updated.
	missingPath := writeNoteFile(t, vault, "missing.md", `---
id: c
type: reference
---
`)
	touchFile(t, missingPath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"stale.md", "fresh.md", "missing.md"})
	require.NoError(t, err)
	require.Len(t, drifts, 1, "only the stale note appears")
	assert.Equal(t, "stale.md", drifts[0].Path)
	assert.Equal(t, "a", drifts[0].NoteID)
}

// TestDetectVMUpdatedDrift_UnquotedYAMLTimestampParsedAsTime — when
// vm_updated is written WITHOUT YAML quotes (e.g. by hand-editing or
// a non-vaultmind tool), yaml.v3 unmarshals the value as time.Time
// rather than string. The detector accepts BOTH forms; without this
// fallback it would silently miss real drift on hand-edited files.
// Vaultmind itself always writes the quoted form because the canonical
// SSOT format contains a colon (which yaml.v3 auto-quotes), but the
// detector cannot assume the producer was vaultmind.
func TestDetectVMUpdatedDrift_UnquotedYAMLTimestampParsedAsTime(t *testing.T) {
	vault := t.TempDir()
	// Note: vm_updated is UNQUOTED here. yaml.v3 will parse this as
	// time.Time, not string.
	notePath := writeNoteFile(t, vault, "unquoted.md", `---
id: ref-unquoted
type: reference
title: Unquoted
vm_updated: 2026-01-01T00:00:00Z
---
body
`)
	touchFile(t, notePath, time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))

	drifts, err := query.DetectVMUpdatedDrift(vault, []string{"unquoted.md"})
	require.NoError(t, err)
	require.Len(t, drifts, 1, "unquoted RFC3339 must be detected as time.Time and compared")
	assert.Equal(t, "2026-01-01T00:00:00Z", drifts[0].VMUpdated,
		"the time.Time fallback re-formats to canonical SSOT for stable output")
}

// TestDetectVMUpdatedDrift_UsesSchemaSSOT — sanity belt: the format
// the detector parses MUST be the same constant used by every write
// site (mutator auto-bump, fix command, template, initvault). If this
// constant ever drifts, the detector silently stops detecting.
func TestDetectVMUpdatedDrift_UsesSchemaSSOT(t *testing.T) {
	assert.Equal(t, "2006-01-02T15:04:05Z", schema.VMUpdatedFormat,
		"detector parse format MUST match schema.VMUpdatedFormat SSOT")
}
