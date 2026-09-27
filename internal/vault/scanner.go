package vault

import (
	"fmt"
	"io/fs"
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
}

// Scan walks the vault directory and returns all .md files,
// excluding directories that match any of the exclude patterns.
//
// Symlinks are never followed — see the guard below for why.
func Scan(vaultRoot string, excludes []string) (ScanResult, error) {
	absRoot, err := filepath.Abs(vaultRoot)
	if err != nil {
		return ScanResult{}, fmt.Errorf("resolving vault root: %w", err)
	}

	skip := newExcludeSet(excludes)

	var files []ScannedFile
	var skippedSymlinks []string

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
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

	return ScanResult{Files: files, SkippedSymlinks: skippedSymlinks}, nil
}
