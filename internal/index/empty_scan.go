package index

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/vault"
)

// refuseWipeOnEmptyScan stops the orphan sweep when the walk saw nothing and
// the index already holds notes.
//
// The decision is scan.SawNothing, not "no indexable files". SawNothing is a
// root that is not a walkable directory, or a walk that visited no entry
// under it. That is the symlink wipe (WalkDir does not descend into a
// symlink root) and a directory that is actually empty. A walk that saw
// entries and found no notes — README, .trash, episodes, excluded or not —
// is a vault that was emptied, and the sweep proceeds.
//
// There is no override. To drop the index on purpose, delete the database
// this process opened (dbPath, which is index.db_path) and run index again.
func refuseWipeOnEmptyScan(db *DB, scan vault.ScanResult, dbPath string) error {
	if !scan.SawNothing() {
		return nil
	}
	var indexed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&indexed); err != nil {
		return fmt.Errorf("counting indexed notes: %w", err)
	}
	if indexed == 0 {
		return nil
	}
	return fmt.Errorf("refusing to delete %d indexed note(s): vault root %s looked empty or unwalkable; nothing was deleted. If you emptied the vault on purpose, delete %s and run vaultmind index again", indexed, scan.Root, dbPath)
}
