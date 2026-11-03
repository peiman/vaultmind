package cmd

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identitySignKeeperCmd = MustNewCommand(commands.IdentitySignKeeperMetadata, runIdentitySignKeeper)

func init() {
	identityCmd.AddCommand(identitySignKeeperCmd)
	setupCommandConfig(identitySignKeeperCmd)
}

func runIdentitySignKeeper(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignkeeperSignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	docJSON, err := readKeeperJSON(cmd)
	if err != nil {
		return err
	}
	stamp := getConfigValueWithFlags[bool](cmd, "stamp", config.KeyAppIdentitysignkeeperStamp)
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignKeeper(cmd.OutOrStdout(), client, docJSON, stamp, time.Now, rand.Reader)
}

// readKeeperJSON reads the document from --file when set, else from stdin.
func readKeeperJSON(cmd *cobra.Command) ([]byte, error) {
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppIdentitysignkeeperFile)
	if filePath != "" {
		b, err := os.ReadFile(filePath) //nolint:gosec // nosemgrep: go-path-traversal -- the operator names the document to sign; it is only read
		if err != nil {
			return nil, fmt.Errorf("reading keeper document file: %w", err)
		}
		return b, nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading keeper document from stdin: %w", err)
	}
	return b, nil
}
