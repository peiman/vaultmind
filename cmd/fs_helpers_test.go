package cmd

import (
	"os"
	"path/filepath"
)

// writeFileAll is a tiny test helper: mkdir -p + write the file. Kept in
// a non-underscored file so it's reusable across all test files without
// re-declaration.
func writeFileAll(root, relPath, content string) error {
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}
