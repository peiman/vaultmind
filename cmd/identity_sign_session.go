package cmd

import (
	"fmt"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identitySignSessionCmd = MustNewCommand(commands.IdentitySignSessionMetadata, runIdentitySignSession)

func init() {
	identityCmd.AddCommand(identitySignSessionCmd)
	setupCommandConfig(identitySignSessionCmd)
}

func runIdentitySignSession(cmd *cobra.Command, _ []string) error {
	challenge := getConfigValueWithFlags[string](cmd, "challenge", config.KeyAppIdentitysignsessionChallenge)
	networkID := getConfigValueWithFlags[string](cmd, "network-id", config.KeyAppIdentitysignsessionNetworkId)
	sockPath, err := sessionSignerSocket(cmd)
	if err != nil {
		return err
	}
	now := time.Now()
	binding, err := resolveOwnSessionBinding(now)
	if err != nil {
		return err
	}
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignSession(cmd.OutOrStdout(), client, binding, challenge, networkID, now)
}

func sessionSignerSocket(cmd *cobra.Command) (string, error) {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignsessionSignerSocket)
	if sockPath != "" {
		return sockPath, nil
	}
	p, err := defaultSignerSocketPath()
	if err != nil {
		return "", fmt.Errorf("resolving signer socket path: %w", err)
	}
	return p, nil
}
