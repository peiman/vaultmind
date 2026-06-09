package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identitySignRegistryCmd = MustNewCommand(commands.IdentitySignRegistryMetadata, runIdentitySignRegistry)

func init() {
	identityCmd.AddCommand(identitySignRegistryCmd)
	setupCommandConfig(identitySignRegistryCmd)
}

func runIdentitySignRegistry(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignregistrySignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	registryJSON, err := readRegistryJSON(cmd)
	if err != nil {
		return err
	}
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignRegistry(cmd.OutOrStdout(), client, registryJSON)
}

// readRegistryJSON reads the registry from --file when set, else from stdin.
func readRegistryJSON(cmd *cobra.Command) ([]byte, error) {
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppIdentitysignregistryFile)
	if filePath != "" {
		b, err := os.ReadFile(filePath) //nolint:gosec // operator-provided registry path
		if err != nil {
			return nil, fmt.Errorf("reading registry file: %w", err)
		}
		return b, nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading registry from stdin: %w", err)
	}
	return b, nil
}
