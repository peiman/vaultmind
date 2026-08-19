package query_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bankVault indexes notes with explicit `created` dates.
func bankVault(t *testing.T, created map[string]string) *index.DB {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  reference:\n    required: [title]\n"), 0o644))
	for name, date := range created {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"),
			[]byte(fmt.Sprintf("---\nid: ref-%s\ntype: reference\ntitle: %s\ncreated: %s\n---\nBody.\n",
				name, name, date)), 0o644))
	}
	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func day(offset int) string {
	return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02")
}

func TestBankRate_CountsOnlyNotesInsideTheWindow(t *testing.T) {
	db := bankVault(t, map[string]string{
		"fresh":  day(-1),
		"recent": day(-6),
		"old":    day(-40),
	})

	rate, err := query.BankRateSince(db, 7)
	require.NoError(t, err)
	assert.Equal(t, 2, rate.Added, "the 40-day-old note is outside a 7-day window")
	assert.Equal(t, 3, rate.Total, "total is the whole vault, so the rate reads against its size")
}

// The boundary is the reason this test exists: an off-by-one here would quietly
// shift every weekly number, and nobody would notice because the shape looks right.
func TestBankRate_IncludesTheBoundaryDay(t *testing.T) {
	db := bankVault(t, map[string]string{"onTheEdge": day(-7)})
	rate, err := query.BankRateSince(db, 7)
	require.NoError(t, err)
	assert.Equal(t, 1, rate.Added, "a note created exactly windowDays ago is inside the window")
}

// The number this vault actually produces today: zero. It must be reachable
// and unremarkable, not an error or an empty struct a caller has to interpret.
func TestBankRate_ZeroIsAValidAnswer(t *testing.T) {
	db := bankVault(t, map[string]string{"ancient": day(-400)})
	rate, err := query.BankRateSince(db, 7)
	require.NoError(t, err)
	assert.Equal(t, 0, rate.Added)
	assert.Equal(t, 1, rate.Total)
}

// A note with no `created` cannot be dated, and guessing would inflate the
// number in the flattering direction.
func TestBankRate_UndatedNotesAreNotCounted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte("types: {}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "undated.md"),
		[]byte("---\nid: ref-undated\ntype: reference\ntitle: Undated\n---\nBody.\n"), 0o644))
	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rate, err := query.BankRateSince(db, 7)
	require.NoError(t, err)
	assert.Equal(t, 0, rate.Added)
	assert.Equal(t, 1, rate.Total)
}

func TestBankRate_RejectsANonPositiveWindow(t *testing.T) {
	db := bankVault(t, map[string]string{"n": day(-1)})
	_, err := query.BankRateSince(db, 0)
	assert.Error(t, err)
}
