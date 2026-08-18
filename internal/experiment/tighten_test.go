package experiment_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	_ "modernc.org/sqlite"
)

func TestOpen_TightensExisting0644(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "experiments.db")

	// Create a database the old way (0644)
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	rawDB.SetMaxOpenConns(1)
	_, _ = rawDB.Exec("PRAGMA journal_mode=WAL")
	_, _ = rawDB.Exec("PRAGMA foreign_keys=ON")
	_, _ = rawDB.Exec("PRAGMA user_version = 7")
	rawDB.Close()

	// Ensure it's 0644
	os.Chmod(dbPath, 0o644)
	info, _ := os.Stat(dbPath)
	fmt.Printf("Before: %s\n", info.Mode())

	// Now open with experiment.Open — should tighten
	db, err := experiment.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	info, _ = os.Stat(dbPath)
	fmt.Printf("After:  %s\n", info.Mode())

	if info.Mode().Perm() != 0o600 {
		t.Errorf("Expected 0600, got %o", info.Mode().Perm())
	}
}
