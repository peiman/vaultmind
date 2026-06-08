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

var identitySignEnvelopeCmd = MustNewCommand(commands.IdentitySignEnvelopeMetadata, runIdentitySignEnvelope)

func init() {
	identityCmd.AddCommand(identitySignEnvelopeCmd)
	setupCommandConfig(identitySignEnvelopeCmd)
}

func runIdentitySignEnvelope(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignenvelopeSignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	fromPubKey := getConfigValueWithFlags[string](cmd, "from-pubkey", config.KeyAppIdentitysignenvelopeFromPubkey)
	envelopeJSON, err := readEnvelopeJSON(cmd)
	if err != nil {
		return err
	}
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignEnvelope(cmd.OutOrStdout(), client, envelopeJSON, fromPubKey)
}

// readEnvelopeJSON reads the envelope from --file when set, else from stdin.
func readEnvelopeJSON(cmd *cobra.Command) ([]byte, error) {
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppIdentitysignenvelopeFile)
	if filePath != "" {
		b, err := os.ReadFile(filePath) //nolint:gosec // operator-provided envelope path
		if err != nil {
			return nil, fmt.Errorf("reading envelope file: %w", err)
		}
		return b, nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading envelope from stdin: %w", err)
	}
	return b, nil
}
