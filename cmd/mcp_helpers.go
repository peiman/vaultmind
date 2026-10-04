package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/mcpserver"
)

// mcpServerConfig resolves the server's vaults to absolute paths, so a
// tool's command finds them whatever directory the client started the
// server in, and refuses a path that is not a vault at start rather than on
// the first tool call.
func mcpServerConfig(single, list string) (mcpserver.Config, error) {
	paths, err := resolveAskVaultPaths(single, list)
	if err != nil {
		return mcpserver.Config{}, err
	}
	for i, p := range paths {
		if paths[i], err = filepath.Abs(p); err != nil {
			return mcpserver.Config{}, fmt.Errorf("resolving vault %s: %w", p, err)
		}
	}
	if len(paths) == 1 && !cmdutil.IsVaultRoot(paths[0]) {
		return mcpserver.Config{}, fmt.Errorf("%s is not a vault; create one with: vaultmind init %s", paths[0], paths[0])
	}
	if err := requireRealVaults(paths); err != nil {
		return mcpserver.Config{}, err
	}
	bin, err := os.Executable()
	if err != nil {
		return mcpserver.Config{}, fmt.Errorf("finding the vaultmind binary to run tools with: %w", err)
	}
	cfg := mcpserver.Config{Vault: paths[0], Version: Version, Run: mcpserver.ExecRunner(bin)}
	if len(paths) > 1 {
		cfg.Vaults = paths
	}
	return cfg, nil
}
