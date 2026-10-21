package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/vault"
)

// ProjectConfig keeps the plugin server available before setup. Calls explain
// the missing vault, then work as soon as setup creates ./knowledge. Explicit
// CLI vault flags continue to use the strict non-plugin configuration path.
func ProjectConfig(project, version, bin string) (Config, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return Config{}, fmt.Errorf("resolving plugin project: %w", err)
	}
	target := filepath.Join(project, "knowledge")
	for _, name := range []string{"knowledge", "vaultmind-vault", "vaultmind-identity", "."} {
		if candidate := filepath.Join(project, name); projectVaultExists(candidate) {
			target = candidate
			break
		}
	}
	run := ExecRunner(bin)
	return Config{Vault: target, Version: version, NoVault: !projectVaultExists(target), Run: func(ctx context.Context, args []string) ([]byte, error) {
		if !projectVaultExists(target) {
			return nil, fmt.Errorf("no vault in this project; run /vaultmind:vaultmind-setup")
		}
		return run(ctx, args)
	}}, nil
}

func projectVaultExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(vault.ConfigRelPath)))
	return err == nil && !info.IsDir()
}
