package cmd

import (
	"bytes"
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/spf13/cobra"
)

var identityEnrollAddCmd = MustNewCommand(commands.IdentityEnrollAddMetadata, runIdentityEnrollAdd)

func init() {
	identityCmd.AddCommand(identityEnrollAddCmd)
	setupCommandConfig(identityEnrollAddCmd)
}

func runIdentityEnrollAdd(cmd *cobra.Command, _ []string) error {
	requestReader, err := openEnrollAddRequest(cmd)
	if err != nil {
		return err
	}
	registryBytes, err := readEnrollAddRegistry(cmd)
	if err != nil {
		return err
	}
	cfg := identitycli.EnrollAddConfig{
		RootPubKeyB64:   getConfigValueWithFlags[string](cmd, "root-pubkey", config.KeyAppIdentityenrolladdRootPubkey),
		NetworkID:       getConfigValueWithFlags[string](cmd, "network-id", config.KeyAppIdentityenrolladdNetworkId),
		RegistryInput:   registryBytes,
		ValiditySeconds: getConfigValueWithFlags[string](cmd, "validity-seconds", config.KeyAppIdentityenrolladdValiditySeconds),
		OriginDaemons:   getConfigValueWithFlags[string](cmd, "origin-daemon", config.KeyAppIdentityenrolladdOriginDaemon),
	}
	return identitycli.EnrollAdd(cmd.OutOrStdout(), cmd.ErrOrStderr(), requestReader, cfg)
}

// openEnrollAddRequest resolves the signed enrollment request to a reader: the
// --request file when set (and not "-"), else stdin.
func openEnrollAddRequest(cmd *cobra.Command) (*bytes.Reader, error) {
	path := getConfigValueWithFlags[string](cmd, "request", config.KeyAppIdentityenrolladdRequest)
	if path != "" && path != "-" {
		b, err := os.ReadFile(path) //nolint:gosec // operator-provided request path
		if err != nil {
			return nil, fmt.Errorf("reading enrollment request file: %w", err)
		}
		return bytes.NewReader(b), nil
	}
	b, err := readAllFrom(cmd)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// readEnrollAddRegistry reads the --registry file when set, else returns nil (a
// fresh registry — the business layer treats nil/empty as epoch 0).
func readEnrollAddRegistry(cmd *cobra.Command) ([]byte, error) {
	path := getConfigValueWithFlags[string](cmd, "registry", config.KeyAppIdentityenrolladdRegistry)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // operator-provided registry path
	if err != nil {
		return nil, fmt.Errorf("reading registry file: %w", err)
	}
	return b, nil
}

// readAllFrom reads all of the command's stdin.
func readAllFrom(cmd *cobra.Command) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(cmd.InOrStdin()); err != nil {
		return nil, fmt.Errorf("reading enrollment request from stdin: %w", err)
	}
	return buf.Bytes(), nil
}
