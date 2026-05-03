package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_CleanVault(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	assert.Greater(t, result.FilesChecked, 0)
	assert.Greater(t, result.Valid, 0)
}

func TestValidate_DetectsMissingRequiredField(t *testing.T) {
	db := openTestDB(t)

	// Insert a project note without status
	rec := index.NoteRecord{
		ID: "proj-no-status", Path: "projects/no-status.md", Type: "project",
		Title: "No Status", Hash: "abc", MTime: 1, IsDomain: true,
		// Status intentionally empty
	}
	require.NoError(t, index.StoreNote(db, rec))

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"project": {Required: []string{"status", "title"}, Statuses: []string{"active", "paused"}},
	})

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	var found bool
	for _, issue := range result.Issues {
		if issue.Rule == "missing_required_field" && issue.Field == "status" {
			found = true
			assert.Equal(t, "error", issue.Severity)
		}
	}
	assert.True(t, found, "should detect missing required field 'status'")
}

func TestValidate_DetectsInvalidStatus(t *testing.T) {
	db := openTestDB(t)

	rec := index.NoteRecord{
		ID: "proj-bad", Path: "projects/bad.md", Type: "project",
		Title: "Bad Status", Status: "invalid_xyz", Hash: "abc", MTime: 1, IsDomain: true,
	}
	require.NoError(t, index.StoreNote(db, rec))

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"project": {Required: []string{"status"}, Statuses: []string{"active", "paused"}},
	})

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	var found bool
	for _, issue := range result.Issues {
		if issue.Rule == "invalid_status" {
			found = true
			assert.Equal(t, "warning", issue.Severity)
		}
	}
	assert.True(t, found, "should detect invalid status")
}

func TestValidate_DetectsUnknownType(t *testing.T) {
	db := openTestDB(t)

	rec := index.NoteRecord{
		ID: "unknown-type", Path: "misc/unknown.md", Type: "unknown_xyz",
		Title: "Unknown", Hash: "abc", MTime: 1, IsDomain: true,
	}
	require.NoError(t, index.StoreNote(db, rec))

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	var found bool
	for _, issue := range result.Issues {
		if issue.Rule == "unknown_type" {
			found = true
			assert.Equal(t, "warning", issue.Severity)
		}
	}
	assert.True(t, found, "should detect unknown type")
}

// TestValidate_AliasSatisfiesRequiredField — DB-backed validate must
// honor schema.aliases the same way validate_live does. Without this,
// migrating users (e.g. shahname-rts: `last_updated` instead of `updated`)
// see spurious missing_required_field errors after indexing — the
// silent-failure-across-layers shape that the close-at-the-right-layer
// arc warned against.
func TestValidate_AliasSatisfiesRequiredField(t *testing.T) {
	db := openTestDB(t)

	// Note carries `last_updated` in frontmatter_kv (the user's existing
	// field name) but not the canonical `updated`. With aliasing in the
	// registry, validate must treat `last_updated` as satisfying `updated`.
	rec := index.NoteRecord{
		ID: "research-foo", Path: "research/foo.md", Type: "research",
		Title: "Foo", Hash: "abc", MTime: 1, IsDomain: true,
		ExtraKV: map[string]interface{}{
			"last_updated": "2026-05-01",
		},
	}
	require.NoError(t, index.StoreNote(db, rec))

	aliases := map[string][]string{
		"updated": {"last_updated"},
	}
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"research": {Required: []string{"updated"}},
	}, aliases)

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	for _, issue := range result.Issues {
		if issue.Rule == "missing_required_field" && issue.Field == "updated" {
			t.Fatalf("alias `last_updated` should satisfy required `updated`; got %+v", issue)
		}
	}
}

// TestValidate_NoAlias_BackwardCompat — without an alias config, the
// missing-required-field rule still fires on the canonical name.
// Pins the backward-compat contract.
func TestValidate_NoAlias_BackwardCompat(t *testing.T) {
	db := openTestDB(t)

	rec := index.NoteRecord{
		ID: "research-bar", Path: "research/bar.md", Type: "research",
		Title: "Bar", Hash: "abc", MTime: 1, IsDomain: true,
		ExtraKV: map[string]interface{}{
			"last_updated": "2026-05-01",
		},
	}
	require.NoError(t, index.StoreNote(db, rec))

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"research": {Required: []string{"updated"}},
	})

	result, err := query.Validate(db, reg)
	require.NoError(t, err)

	var found bool
	for _, issue := range result.Issues {
		if issue.Rule == "missing_required_field" && issue.Field == "updated" {
			found = true
		}
	}
	assert.True(t, found, "without alias config, canonical `updated` should be reported missing")
}

// TestDoctor_PopulatesMissingRequiredFields — when a registry is passed,
// Doctor calls Validate and surfaces the count of missing type-required
// fields. The 2026-05-04 fix made the previously-dead MissingRequiredFields
// counter actually populate (silent-failure shape per the pre-push code
// review). The validator iterates td.Required only — vaultmind-owned
// fields (created, vm_updated) are auto-maintained by vaultmind itself,
// not user-required, so they're not counted here.
func TestDoctor_PopulatesMissingRequiredFields(t *testing.T) {
	db := openTestDB(t)

	// Note: missing url (type-required for "source"). Expect counter == 1.
	require.NoError(t, index.StoreNote(db, index.NoteRecord{
		ID: "src-no-url", Path: "n1.md", Type: "source",
		Title: "X", Hash: "abc", MTime: 1, IsDomain: true,
	}))

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"source": {Required: []string{"url"}},
	})

	// With registry: counter populated.
	result, err := query.Doctor(db, "/vault", reg)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Issues.MissingRequiredFields,
		"doctor should count the missing type-required url field")

	// Without registry: counter stays 0 (backward-compat with pre-fix callers).
	resultNil, err := query.Doctor(db, "/vault", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, resultNil.Issues.MissingRequiredFields,
		"doctor with nil registry should not populate the counter (caller hasn't loaded a registry)")
}

// openTestDB creates a fresh empty DB for unit tests.
func openTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
