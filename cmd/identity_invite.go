package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identityInviteCmd = MustNewCommand(commands.IdentityInviteMetadata, runIdentityInvite)

func init() {
	identityCmd.AddCommand(identityInviteCmd)
	setupCommandConfig(identityInviteCmd)
}

func runIdentityInvite(cmd *cobra.Command, _ []string) error {
	rootPubKey := getConfigValueWithFlags[string](cmd, "root-pubkey", config.KeyAppIdentityinviteRootPubkey)
	relay := getConfigValueWithFlags[string](cmd, "relay", config.KeyAppIdentityinviteRelay)
	return identitycli.Invite(cmd.OutOrStdout(), rootPubKey, relay)
}
