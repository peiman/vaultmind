package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// NoteExtension is the extension of a note file; only these are indexed.
const NoteExtension = ".md"

// ScannedFile represents a discovered .md file in the vault.
type ScannedFile struct {
	RelPath string    // Vault-relative path (e.g., "concepts/act-r.md")
	AbsPath string    // Absolute filesystem path
	ModTime time.Time // Last modification time
}

// ScanResult is what a walk found: the notes to index, and what was passed over
// for a reason the caller has to answer for.
//
// A struct rather than extra return values because "skipped, and why" is a
// growing category — and a plain []ScannedFile let a whole class of skip go
// unmentioned, which is how a note vanishes from an index in silence.
type ScanResult struct {
	Files []ScannedFile

	// SkippedSymlinks holds vault-relative paths of *.md symlinks that were not
	// followed. Callers are expected to REPORT these, not drop them: from the
	// outside, a skipped note and a missing note look identical.
	SkippedSymlinks []string

	// Entries is how many filesystem entries the walk visited under the root.
	// The root itself is not one of them. Excluded files and directories count:
	// seeing them is evidence the vault is there. A skipped directory counts
	// once; the walk does not visit what is inside it.
	Entries int

	// Root is the path the walk used, after ResolveRoot.
	Root string

	// Walkable is true when Root is a real directory: it exists, it is not a
	// symlink (a broken one included), and it is a directory.
	Walkable bool
}

// SawNothing is the one answer the wipe guard and doctor share (Principle 7).
// True when the root is not a directory the walk can enter, or the walk
// visited nothing under it. A walk that saw entries — a README, .trash,
// episodes, excluded or not — saw a vault; zero indexable notes is then a
// real purge. The zero value is SawNothing, so an unset result cannot
// authorize a wipe.
func (s ScanResult) SawNothing() bool {
	return !s.Walkable || s.Entries == 0
}

// Scan walks the vault directory and returns all .md files,
// excluding directories that match any of the exclude patterns.
//
// The root the operator named is resolved first (ResolveRoot). WalkDir does
// not descend into a root that is a symlink, and an empty result is what
// made index delete every note. Symlinks inside the vault are never
// followed — see the guard below for why.
func Scan(vaultRoot string, excludes []string) (ScanResult, error) {
	absRoot, err := ResolveRoot(vaultRoot)
	if err != nil {
		return ScanResult{}, err
	}

	skip := newExcludeSet(excludes)

	var files []ScannedFile
	var skippedSymlinks []string
	var entries int

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if visitedUnderRoot(absRoot, path) {
			entries++
		}
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			// By name ("templates", ".obsidian") or by relative path prefix
			// ("archive/old"); the root is checked by name, as it always was.
			relDir, relErr := filepath.Rel(absRoot, path)
			if relErr != nil {
				relDir = ""
			}
			if skip.dir(d.Name(), relDir) {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != NoteExtension {
			return nil
		}

		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return fmt.Errorf("computing relative path: %w", relErr)
		}

		// Never follow a symlink; SkipSymlink carries the whole reasoning.
		if rel, skip := SkipSymlink(absRoot, path, d); skip {
			skippedSymlinks = append(skippedSymlinks, rel)
			return nil
		}

		// Exclude files too, not just directories: a basename match (e.g.
		// "README.md" — vault meta, not a knowledge note) or an exact
		// vault-relative path. Without this, a vault's own README indexed as a
		// note and polluted every query's results.
		if skip.file(relPath) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("getting file info for %s: %w", relPath, infoErr)
		}

		files = append(files, ScannedFile{
			RelPath: relPath,
			AbsPath: path,
			ModTime: info.ModTime(),
		})

		return nil
	})

	if err != nil {
		return ScanResult{}, fmt.Errorf("scanning vault %s: %w", absRoot, err)
	}

	return ScanResult{
		Files:           files,
		SkippedSymlinks: skippedSymlinks,
		Entries:         entries,
		Root:            absRoot,
		Walkable:        rootWalkable(absRoot),
	}, nil
}

// visitedUnderRoot reports a walk visit that is not the root itself.
func visitedUnderRoot(absRoot, path string) bool {
	rel, err := filepath.Rel(absRoot, path)
	return err == nil && rel != "."
}

// rootWalkable reports a directory WalkDir will descend into. A symlink,
// including a broken one, is not: WalkDir lstats its root and stops.
func rootWalkable(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}
