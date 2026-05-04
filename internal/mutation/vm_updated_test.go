package mutation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vm_updated auto-maintenance — slice 2 of the 2026-05-04 schema
// foundation chain. Vaultmind owns vm_updated; the mutator is one of
// the auto-write sites that keeps it current. Per principle 9
// (automated enforcement), every successful mutation bumps the field
// — no honor system, no relying on the human to remember.
//
// Tests pin the contract:
//   - Every Op (Set, Unset, Merge, Normalize) bumps vm_updated.
//   - Bump uses RFC3339 datetime (sub-day precision) so the
//     `mtime > vm_updated` comparison can detect "human edited
//     since vaultmind processed" without false positives within
//     the same calendar day.
//   - Idempotent operations still bump (semantics: vaultmind LOOKED
//     at this on date X — even no-op normalizations).
//   - Non-domain notes (no id+type) get nothing — vaultmind doesn't
//     track non-domain content.
//   - Dry-run shows the bump in the diff but doesn't persist.
//   - Pre-existing vm_updated is overwritten with today's value.

const vmUpdatedRFC3339 = "2006-01-02T15:04:05Z"

func todayRFC3339Prefix(t *testing.T) string {
	t.Helper()
	return time.Now().UTC().Format("2006-01-02")
}

func readVMUpdated(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "vm_updated:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "vm_updated:"))
			// YAML auto-quotes strings containing colons (RFC3339 has them).
			// Strip surrounding double or single quotes for parse comparisons.
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

func TestMutator_Run_BumpsVMUpdatedOnSet(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	got := readVMUpdated(t, string(content))
	assert.NotEmpty(t, got, "OpSet must add vm_updated")
	assert.Contains(t, got, todayRFC3339Prefix(t), "vm_updated must be today")
	// Verify the format is RFC3339 (second-precision UTC datetime),
	// not date-only — the comparison against file mtime requires
	// sub-day precision to avoid false-positive "edited since processed".
	_, parseErr := time.Parse(vmUpdatedRFC3339, got)
	assert.NoError(t, parseErr, "vm_updated must be RFC3339 (got %q)", got)
}

func TestMutator_Run_BumpsVMUpdatedOnUnset(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpUnset, Target: "projects/test-project.md", Key: "tags",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, readVMUpdated(t, string(content)), todayRFC3339Prefix(t),
		"OpUnset must bump vm_updated to today")
}

func TestMutator_Run_BumpsVMUpdatedOnMerge(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op:     mutation.OpMerge,
		Target: "projects/test-project.md",
		Fields: map[string]interface{}{"status": "paused", "owner_id": "person-x"},
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, readVMUpdated(t, string(content)), todayRFC3339Prefix(t),
		"OpMerge must bump vm_updated to today")
}

func TestMutator_Run_BumpsVMUpdatedOnNormalize(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpNormalize, Target: "projects/test-project.md",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	// Even a no-op-ish normalize semantically means "vaultmind processed
	// this on date X." That's the value of vm_updated — it answers
	// "when did vaultmind last LOOK at this," not just "when did
	// vaultmind change content."
	assert.Contains(t, readVMUpdated(t, string(content)), todayRFC3339Prefix(t),
		"OpNormalize must bump vm_updated even on idempotent ops")
}

func TestMutator_Run_VMUpdatedOverwritesExisting(t *testing.T) {
	vaultPath := setupTestVault(t)
	// Replace the test fixture with a note that ALREADY has an old
	// vm_updated — the mutator must overwrite, not skip.
	noteContent := "---\nid: proj-test\ntype: project\nstatus: active\ntitle: T\nvm_updated: 2024-01-01T00:00:00Z\n---\nbody\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/test-project.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	s := string(content)
	assert.NotContains(t, s, "2024-01-01", "stale vm_updated must be overwritten")
	assert.Contains(t, readVMUpdated(t, s), todayRFC3339Prefix(t))
}

func TestMutator_Run_DryRunDoesNotPersistVMUpdated(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)
	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
		DryRun: true, Diff: true,
	})
	require.NoError(t, err)

	// Diff shows the bump (so the user sees vaultmind's intent before
	// the apply step) — but the on-disk file is untouched.
	assert.Contains(t, result.Diff, "vm_updated", "dry-run diff should preview the vm_updated bump")
	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	// The fixture had no vm_updated; dry-run must not add one.
	assert.NotContains(t, string(content), "vm_updated:",
		"dry-run must not persist vm_updated on disk")
}

func TestMutator_Run_NonDomainNoteSkipsBump(t *testing.T) {
	vaultPath := setupTestVault(t)
	// Non-domain note: has no id and no type. The mutator's
	// resolveTarget would fail-classify and the request might error,
	// but if it ever reaches the apply step on a non-domain mapping,
	// vm_updated must NOT be added (vaultmind doesn't track non-
	// domain content). This test pins the safety property: only
	// domain notes get vaultmind-owned auto-fields.
	//
	// Implementation note: today, the mutator's resolveTarget would
	// reject a non-domain note path before reaching the apply step;
	// this test validates that when it does reach the apply step
	// (via the IsDomain guard inside Run), the bump is gated.
	noteContent := "---\nfree_form: true\n---\nstray markdown\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/stray.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/stray.md",
		Key: "free_form", Value: false,
	})
	// Resolver rejects non-domain note up front — that's the existing
	// contract (validate.go's IsDomain check). Verify vm_updated was
	// NOT added by ANY codepath as a side effect.
	require.Error(t, err)
	content, _ := os.ReadFile(filepath.Join(vaultPath, "projects/stray.md"))
	assert.NotContains(t, string(content), "vm_updated:",
		"non-domain note must never receive vm_updated from the mutator")
}
