// ckeletin:allow-custom-command
//
// The `identity` parent is a custom group command, not a thin metadata-driven
// leaf: it has no ConfigPrefix/options (its behavior is "show help") and carries
// a single local bool flag (--print-instructions) that prints the embedded mesh
// onboarding quick-start. The whitelist opts it out of the ADR-001 thin-command
// checks (no metadata file, no NewCommand helper) — the same escape hatch
// zz_catalog.go uses for catalog wiring.
package cmd

import (
	"github.com/peiman/vaultmind/internal/onboard"
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
	// networkAnchorFilename is the 0600 file `identity enroll` pins the
	// OOB-confirmed network trust anchor to (beside the signer key in the XDG data
	// dir). A later `doctor` slice authenticates registry verification against it.
	networkAnchorFilename = "network-roots.json"

	// identityPrintInstructionsFlag prints the embedded mesh onboarding
	// quick-start (the admin + member journeys for joining a Contract-B network)
	// instead of the command's help. Symmetric with `init --print-instructions`.
	identityPrintInstructionsFlag = "print-instructions"
	// identityPrintInstructionsDesc is the --print-instructions flag's help text.
	identityPrintInstructionsDesc = "Print the mesh onboarding quick-start (admin + member journeys) and exit"
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
	RunE: runIdentity,
}

func init() {
	identityCmd.Flags().Bool(identityPrintInstructionsFlag, false, identityPrintInstructionsDesc)
	RootCmd.AddCommand(identityCmd)
}

// runIdentity is the parent command's entrypoint: with --print-instructions it
// prints the embedded mesh onboarding quick-start; otherwise it preserves the
// group's default behavior of showing help (bare `vaultmind identity`).
func runIdentity(cmd *cobra.Command, _ []string) error {
	if printInstructions, _ := cmd.Flags().GetBool(identityPrintInstructionsFlag); printInstructions {
		return onboard.PrintMeshQuickStart(cmd.OutOrStdout())
	}
	return cmd.Help()
}

// defaultSignerKeyPath returns the sealed key-file path under the XDG data dir.
func defaultSignerKeyPath() (string, error) {
	return xdg.DataFile(signerKeyFilename)
}

// defaultSignerSocketPath returns the signer socket path under the XDG state dir.
func defaultSignerSocketPath() (string, error) {
	return xdg.StateFile(signerSocketFilename)
}

// defaultNetworkAnchorPath returns the persisted-trust-anchor path under the XDG
// data dir (beside the sealed signer key). `identity enroll` pins the
// OOB-confirmed network root here; it is NOT a user flag.
func defaultNetworkAnchorPath() (string, error) {
	return xdg.DataFile(networkAnchorFilename)
}
