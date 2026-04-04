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

// openTestDB creates a fresh empty DB for unit tests.
func openTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
