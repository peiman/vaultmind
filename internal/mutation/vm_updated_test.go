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

// TestMutator_Run_OpSetNonDomainErrorsBeforeBump pins the validation
// gate's contract for non-domain notes under OpSet/OpUnset/OpMerge:
// ValidateMutation rejects with not_domain_note BEFORE the apply step,
// so vm_updated never gets written. This is the EARLY-error path.
//
// The OpNormalize path is tested separately (see
// TestMutator_Run_OpNormalizeNonDomainSkipsBump) because OpNormalize
// has an early-return in ValidateMutation that bypasses the IsDomain
// check — the IsDomain guard inside Run is load-bearing for that path.
func TestMutator_Run_OpSetNonDomainErrorsBeforeBump(t *testing.T) {
	vaultPath := setupTestVault(t)
	noteContent := "---\nfree_form: true\n---\nstray markdown\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/stray.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/stray.md",
		Key: "free_form", Value: false,
	})
	require.Error(t, err)
	content, _ := os.ReadFile(filepath.Join(vaultPath, "projects/stray.md"))
	assert.NotContains(t, string(content), "vm_updated:",
		"non-domain note must never receive vm_updated under OpSet (validation rejects)")
}

// TestMutator_Run_OpNormalizeNonDomainSkipsBump pins the LOAD-BEARING
// IsDomain guard inside Run. ValidateMutation has an early-return for
// OpNormalize that bypasses the IsDomain check — so a non-domain note
// passes validation and reaches the bump code. The guard at
// mutator.go:99-105 is the only thing preventing vm_updated from being
// written to non-domain content. Without the guard, normalizing a
// stray markdown file would silently add a vaultmind-owned field —
// violating the four-tier taxonomy contract (vaultmind doesn't write
// to content it doesn't track).
//
// Caught by 2026-05-04 review pass on commit b5ff2ea — the original
// test only covered OpSet (which never reaches the bump because
// validation rejects first). This test covers the path that DOES
// reach the bump.
func TestMutator_Run_OpNormalizeNonDomainSkipsBump(t *testing.T) {
	vaultPath := setupTestVault(t)
	noteContent := "---\nfree_form: true\n---\nstray markdown\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/stray.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	// OpNormalize on a non-domain note: validation passes (early-return),
	// apply runs, but the IsDomain guard prevents the vm_updated bump.
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpNormalize, Target: "projects/stray.md",
	})
	// Normalize on a non-domain note may succeed (validation allows it)
	// or fail downstream (the resolver/parser may still reject malformed
	// frontmatter). Either way: vm_updated must NOT be written.
	_ = err
	content, _ := os.ReadFile(filepath.Join(vaultPath, "projects/stray.md"))
	assert.NotContains(t, string(content), "vm_updated:",
		"non-domain note must never receive vm_updated under OpNormalize (IsDomain guard)")
}
