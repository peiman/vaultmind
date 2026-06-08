package cmd

import (
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

// Contract-B identity custody path/flag constants (SSOT). The CLI is KEYLESS:
// it never opens the key file. The key path exists so `identity init` can SEAL
// the key for the signer, and so docs/operators can locate it — the signing
// path uses only the socket via the signer Client.
const (
	// signerKeyFilename is the sealed ed25519 private-key file the signer loads.
	signerKeyFilename = "identity-signer.key"
	// signerSocketFilename is the 0600 Unix-domain socket the signer listens on.
	signerSocketFilename = "identity-signer.sock"
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Contract-B agent identity: keypair custody + signing",
	Long: `Contract-B agent identity commands.

The ed25519 private key is held by a SEPARATE signer process and reached over a
0600 Unix-domain socket; this CLI is KEYLESS and never opens the key file.

DEV-INTERIM: the signer currently runs as the SAME uid as the CLI. This is the
custody ARCHITECTURE, not real isolation — real isolation needs a dedicated
service uid + launchd + sandbox + Secure-Enclave key wrap (deferred).`,
}

func init() {
	RootCmd.AddCommand(identityCmd)
}

// defaultSignerKeyPath returns the sealed key-file path under the XDG data dir.
func defaultSignerKeyPath() (string, error) {
	return xdg.DataFile(signerKeyFilename)
}

// defaultSignerSocketPath returns the signer socket path under the XDG state dir.
func defaultSignerSocketPath() (string, error) {
	return xdg.StateFile(signerSocketFilename)
}
