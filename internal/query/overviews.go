package query

import (
	"fmt"
	"path"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
)

// A folder overview (a note of type overview) is what the map shows beside a
// folder. On the 2026-10-03 probe, curated one-line overviews lifted an
// agent's first-turn "does this vault cover X?" from 73% (folder names and
// counts) to 97%. Doctor names the folders that would gain from one, and the
// overviews their folders have outgrown.
const (
	overviewType = vault.OverviewType
	// overviewMinNotes is the folder size, in notes directly inside it, at
	// which a missing overview is named.
	overviewMinNotes = 20
	// An overview is stale when at least staleMinChanged of its folder's
	// notes, and at least staleMinShare of them, changed after it.
	staleMinChanged = 10
	staleMinShare   = 0.25
)

// FolderCount is a folder and the notes directly inside it.
type FolderCount struct {
	Folder string `json:"folder"`
	Notes  int    `json:"notes"`
}

// StaleOverview is an overview older than much of its folder.
type StaleOverview struct {
	Overview string `json:"overview"`
	Changed  int    `json:"changed"`
	Notes    int    `json:"notes"`
}

// OverviewHealth is doctor's view of the vault's folder overviews.
type OverviewHealth struct {
	Missing []FolderCount   `json:"missing,omitempty"`
	Stale   []StaleOverview `json:"stale,omitempty"`
}

// CheckOverviews finds folders of overviewMinNotes or more notes without an
// overview, and overviews that most of their folder has moved past. The vault
// root is not a folder here: the map describes the folders in it.
func CheckOverviews(db *index.DB) (*OverviewHealth, error) {
	rows, err := db.Query(`SELECT path, COALESCE(type, ''), mtime FROM notes ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("listing notes for overviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type folder struct {
		overview      string
		overviewMtime int64
		mtimes        []int64
	}
	folders := map[string]*folder{}
	for rows.Next() {
		var p, typ string
		var mtime int64
		if err := rows.Scan(&p, &typ, &mtime); err != nil {
			return nil, fmt.Errorf("reading note row: %w", err)
		}
		dir := path.Dir(p)
		if dir == "." {
			continue
		}
		f := folders[dir]
		if f == nil {
			f = &folder{}
			folders[dir] = f
		}
		if typ == overviewType && (f.overview == "" || p < f.overview) {
			f.overview, f.overviewMtime = p, mtime
			continue
		}
		f.mtimes = append(f.mtimes, mtime)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	h := &OverviewHealth{}
	for dir, f := range folders {
		n := len(f.mtimes)
		if f.overview == "" {
			if n >= overviewMinNotes {
				h.Missing = append(h.Missing, FolderCount{Folder: dir, Notes: n})
			}
			continue
		}
		changed := 0
		for _, m := range f.mtimes {
			if m > f.overviewMtime {
				changed++
			}
		}
		if changed >= staleMinChanged && float64(changed) >= staleMinShare*float64(n) {
			h.Stale = append(h.Stale, StaleOverview{Overview: f.overview, Changed: changed, Notes: n})
		}
	}
	sort.Slice(h.Missing, func(i, j int) bool { return h.Missing[i].Folder < h.Missing[j].Folder })
	sort.Slice(h.Stale, func(i, j int) bool { return h.Stale[i].Overview < h.Stale[j].Overview })
	return h, nil
}
