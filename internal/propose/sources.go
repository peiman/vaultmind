package propose

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/vault"
)

func validateSources(vaultPath, workDir string, sources []string) error {
	for _, source := range sources {
		if err := validateSource(vaultPath, workDir, source); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(vaultPath, workDir, source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source is empty")
	}
	// A Windows path such as C:\notes\a.md has no "://", so it stays a path.
	if strings.Contains(source, "://") {
		return validateSourceURL(source)
	}
	return validateSourcePath(vaultPath, workDir, source)
}

func validateSourceURL(source string) error {
	u, err := url.Parse(source)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("source %q is not a valid URL", source)
	}
	if u.Scheme == "file" {
		if u.Path == "" {
			return fmt.Errorf("source %q is not a valid URL", source)
		}
		return nil
	}
	if u.Host == "" {
		return fmt.Errorf("source %q is not a valid URL", source)
	}
	return nil
}

func validateSourcePath(vaultPath, workDir, source string) error {
	if abs, err := vault.ResolveInside(vaultPath, source); err == nil {
		if ok, dir, statErr := fileExists(abs); statErr == nil && ok && !dir {
			return nil
		} else if statErr == nil && dir {
			return fmt.Errorf("source %q is a directory", source)
		}
	}
	for _, candidate := range candidatePaths(vaultPath, workDir, source) {
		ok, dir, err := fileExists(candidate)
		if err != nil {
			continue
		}
		if dir {
			return fmt.Errorf("source %q is a directory", source)
		}
		if !ok {
			continue
		}
		if insideAny(allowedRoots(vaultPath, workDir), candidate) {
			return nil
		}
		return fmt.Errorf("source %q is outside the vault and the repository", source)
	}
	return fmt.Errorf("source %q does not exist", source)
}

func fileExists(path string) (bool, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}
	if err != nil {
		return false, false, err
	}
	return true, info.IsDir(), nil
}

func candidatePaths(vaultPath, workDir, source string) []string {
	if filepath.IsAbs(source) {
		return []string{filepath.Clean(source)}
	}
	var out []string
	if abs, err := filepath.Abs(filepath.Join(vaultPath, source)); err == nil {
		out = append(out, abs)
	}
	if workDir != "" {
		if abs, err := filepath.Abs(filepath.Join(workDir, source)); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func allowedRoots(vaultPath, workDir string) []string {
	var roots []string
	if abs, err := filepath.Abs(vaultPath); err == nil {
		roots = append(roots, abs)
		if root := gitRoot(abs); root != "" {
			roots = append(roots, root)
		}
	}
	if workDir != "" {
		if abs, err := filepath.Abs(workDir); err == nil {
			if root := gitRoot(abs); root != "" {
				roots = append(roots, root)
			}
		}
	}
	return roots
}

// gitRoot walks parents for a .git directory or a gitlink file.
func gitRoot(start string) string {
	dir := start
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		dir = filepath.Dir(start)
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func insideAny(roots []string, target string) bool {
	for _, root := range roots {
		if pathInside(root, target) {
			return true
		}
	}
	return false
}

func pathInside(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func workDir(req Request) string {
	if req.WorkDir != "" {
		return req.WorkDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
