package index

import "fmt"

// refuseWipeOnEmptyScan stops the orphan sweep when a scan saw no notes but
// the index already holds some.
//
// A scan of zero files is not evidence the operator deleted the vault. It is
// also what a symlinked root used to produce (WalkDir does not descend into
// one) and what an exclude list that matches every note produces. Those are
// indistinguishable from a vault that really was emptied, and the sweep
// deleted every row — embeddings included. The index is kept.
//
// There is no override. The previous contract, a rebuild of an emptied vault
// deleting every note, is the data loss. To drop the index on purpose, delete
// .vaultmind/index.db and run index again. Deleting some notes while others
// remain is still a purge: this returns only when the scan found nothing.
func refuseWipeOnEmptyScan(db *DB, scanned int) error {
	if scanned > 0 {
		return nil
	}
	var indexed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&indexed); err != nil {
		return fmt.Errorf("counting indexed notes: %w", err)
	}
	if indexed == 0 {
		return nil
	}
	return fmt.Errorf("refusing to delete %d indexed note(s): vault scan found no notes, so the vault path looks empty or unreadable; nothing was deleted. If you emptied the vault on purpose, delete .vaultmind/index.db and run vaultmind index again", indexed)
}
