package query

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
)

// changedOnDiskNote is the line printed under a delivered note whose file no
// longer matches the indexed copy the text came from.
const changedOnDiskNote = "(changed on disk since indexed — this text may be out of date; run vaultmind index, or read the file)"

// MarkChangedOnDisk flags every note whose text this answer delivers but whose
// file changed after it was indexed (or was deleted). Ask serves text from the
// index, so without this an edited note comes back with its pre-edit text and
// nothing says so (#407).
//
// Cost is one stat per delivered note. Only when the mtime differs from the
// indexed one is the file read, and only to compare its hash with the stored
// one, so a touch that changed nothing is not flagged. It never reindexes.
func MarkChangedOnDisk(result *AskResult, db *index.DB, vaultRoot string) {
	if result == nil || result.Context == nil {
		return
	}
	pack := result.Context
	if t := pack.Target; t != nil && t.Body != "" && !t.ShownBefore && changedOnDisk(db, vaultRoot, pack.TargetID) {
		t.ChangedOnDisk = true
		result.StaleNotes++
	}
	for i := range pack.Context {
		item := &pack.Context[i]
		if item.Body == "" || item.ShownBefore || !changedOnDisk(db, vaultRoot, item.ID) {
			continue
		}
		item.ChangedOnDisk = true
		result.StaleNotes++
	}
}

// changedOnDisk reports whether the file behind a note differs from the copy
// that was indexed. Anything it cannot determine reads as unchanged: the
// marker is a positive claim and must not be made on a guess.
func changedOnDisk(db *index.DB, vaultRoot, id string) bool {
	var path, hash string
	var mtime int64
	if err := db.QueryRow("SELECT path, hash, mtime FROM notes WHERE id = ?", id).Scan(&path, &hash, &mtime); err != nil {
		log.Debug().Err(err).Str("note_id", id).Msg("freshness check: note not in index")
		return false
	}
	abs, err := vault.ResolveInside(vaultRoot, path)
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("freshness check: path outside vault")
		return false
	}
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("freshness check: stat failed")
		return false
	}
	if info.ModTime().Unix() == mtime {
		return false
	}
	// nosemgrep: go-path-traversal -- abs comes from vault.ResolveInside above, which refuses anything outside the root
	content, err := os.ReadFile(abs) //nolint:gosec // see above
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("freshness check: read failed")
		return false
	}
	return fmt.Sprintf("%x", sha256.Sum256(content)) != hash
}
