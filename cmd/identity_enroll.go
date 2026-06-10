package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identityEnrollCmd = MustNewCommand(commands.IdentityEnrollMetadata, runIdentityEnroll)

func init() {
	identityCmd.AddCommand(identityEnrollCmd)
	setupCommandConfig(identityEnrollCmd)
}

func runIdentityEnroll(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentityenrollSignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	anchorPath, err := defaultNetworkAnchorPath()
	if err != nil {
		return fmt.Errorf("resolving network anchor path: %w", err)
	}
	cfg := identitycli.EnrollConfig{
		InviteTokenOrURL:   getConfigValueWithFlags[string](cmd, "invite", config.KeyAppIdentityenrollInvite),
		DisplayName:        getConfigValueWithFlags[string](cmd, "display-name", config.KeyAppIdentityenrollDisplayName),
		Slug:               getConfigValueWithFlags[string](cmd, "slug", config.KeyAppIdentityenrollSlug),
		PubKeyB64:          getConfigValueWithFlags[string](cmd, "pubkey", config.KeyAppIdentityenrollPubkey),
		TransportPubKeyB64: getConfigValueWithFlags[string](cmd, "transport-pubkey", config.KeyAppIdentityenrollTransportPubkey),
		TransportEndpoint:  getConfigValueWithFlags[string](cmd, "transport-endpoint", config.KeyAppIdentityenrollTransportEndpoint),
		SignerSocket:       sockPath,
		AnchorStorePath:    anchorPath,
		AssumeYes:          getConfigValueWithFlags[bool](cmd, "yes", config.KeyAppIdentityenrollYes),
	}
	return identitycli.Enroll(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), cfg)
}
