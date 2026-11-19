package cmd

import (
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/signhealth"
	"github.com/spf13/cobra"
)

var identitySignHealthCmd = MustNewCommand(commands.IdentitySignHealthMetadata, runIdentitySignHealth)

func init() {
	identityCmd.AddCommand(identitySignHealthCmd)
	setupCommandConfig(identitySignHealthCmd)
}

func runIdentitySignHealth(cmd *cobra.Command, _ []string) error {
	rawMaxAge := getConfigValueWithFlags[string](cmd, "max-age", config.KeyAppIdentitysignhealthMaxAge)
	maxAge, err := parseSignHealthMaxAge(rawMaxAge)
	if err != nil {
		return err
	}
	entries, err := signhealth.Read(time.Now().UTC(), maxAge)
	if err != nil {
		return err
	}
	return renderSignHealth(cmd, entries)
}
