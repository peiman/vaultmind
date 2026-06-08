package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identityInitCmd = MustNewCommand(commands.IdentityInitMetadata, runIdentityInit)

func init() {
	identityCmd.AddCommand(identityInitCmd)
	setupCommandConfig(identityInitCmd)
}

func runIdentityInit(cmd *cobra.Command, _ []string) error {
	keyPath := getConfigValueWithFlags[string](cmd, "signer-key", config.KeyAppIdentityinitSignerKey)
	if keyPath == "" {
		p, err := defaultSignerKeyPath()
		if err != nil {
			return fmt.Errorf("resolving signer key path: %w", err)
		}
		keyPath = p
	}
	return identitycli.Init(cmd.OutOrStdout(), keyPath)
}
