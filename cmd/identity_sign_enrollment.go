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

var identitySignEnrollmentCmd = MustNewCommand(commands.IdentitySignEnrollmentMetadata, runIdentitySignEnrollment)

func init() {
	identityCmd.AddCommand(identitySignEnrollmentCmd)
	setupCommandConfig(identitySignEnrollmentCmd)
}

func runIdentitySignEnrollment(cmd *cobra.Command, _ []string) error {
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignenrollmentSignerSocket)
	if sockPath == "" {
		p, err := defaultSignerSocketPath()
		if err != nil {
			return fmt.Errorf("resolving signer socket path: %w", err)
		}
		sockPath = p
	}
	enrollmentJSON, err := readEnrollmentJSON(cmd)
	if err != nil {
		return err
	}
	client := &signer.Client{SocketPath: sockPath}
	return identitycli.SignEnrollment(cmd.OutOrStdout(), client, enrollmentJSON)
}

// readEnrollmentJSON reads the enrollment request from --file when set, else from
// stdin.
func readEnrollmentJSON(cmd *cobra.Command) ([]byte, error) {
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppIdentitysignenrollmentFile)
	if filePath != "" {
		b, err := os.ReadFile(filePath) //nolint:gosec // operator-provided enrollment path
		if err != nil {
			return nil, fmt.Errorf("reading enrollment file: %w", err)
		}
		return b, nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading enrollment from stdin: %w", err)
	}
	return b, nil
}
