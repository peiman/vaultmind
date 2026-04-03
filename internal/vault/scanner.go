package vault

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
)

// ScannedFile represents a discovered .md file in the vault.
type ScannedFile struct {
	RelPath string    // Vault-relative path (e.g., "concepts/act-r.md")
	AbsPath string    // Absolute filesystem path
	ModTime time.Time // Last modification time
}

// Scan walks the vault directory and returns all .md files,
// excluding directories that match any of the exclude patterns.
func Scan(vaultRoot string, excludes []string) ([]ScannedFile, error) {
	absRoot, err := filepath.Abs(vaultRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving vault root: %w", err)
	}

	excludeSet := make(map[string]bool, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = true
	}

	var files []ScannedFile

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if excludeSet[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}

		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return fmt.Errorf("computing relative path: %w", relErr)
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
		return nil, fmt.Errorf("scanning vault %s: %w", absRoot, err)
	}

	return files, nil
}
