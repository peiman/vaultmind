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

var identitySignCmd = MustNewCommand(commands.IdentitySignMetadata, runIdentitySign)

func init() {
	identityCmd.AddCommand(identitySignCmd)
	setupCommandConfig(identitySignCmd)
}

func runIdentitySign(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignSignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	entryJSON, err := readIdentityEntryJSON(cmd)
	if err != nil {
		return err
	}
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignEntry(cmd.OutOrStdout(), client, entryJSON)
}

// readIdentityEntryJSON reads the entry from --file when set, else from stdin.
func readIdentityEntryJSON(cmd *cobra.Command) ([]byte, error) {
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppIdentitysignFile)
	if filePath != "" {
		b, err := os.ReadFile(filePath) //nolint:gosec // operator-provided entry path
		if err != nil {
			return nil, fmt.Errorf("reading entry file: %w", err)
		}
		return b, nil
	}
	// TODO(contract-b hardening): io.ReadAll here is unbounded — a caller
	// piping a huge stream would consume unbounded memory before the signer
	// even sees the bytes. In the hardening slice, replace with
	// io.LimitReader(cmd.InOrStdin(), maxEntryBytes) where maxEntryBytes
	// matches (or is tighter than) the signer's maxRequestBytes constant.
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading entry from stdin: %w", err)
	}
	return b, nil
}
