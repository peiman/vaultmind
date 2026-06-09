package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignerMetadata defines metadata for the `identity signer` command —
// the keyless custody-signer DAEMON that the sign-* commands connect to.
var IdentitySignerMetadata = config.CommandMetadata{
	Use:   "signer",
	Short: "Run the keyless custody signer daemon (Contract-B)",
	Long: `Run the keyless custody signer in the FOREGROUND. The signer holds the agent's
ed25519 private key in memory and serves signatures over a 0600 Unix-domain
socket, gated by LOCAL_PEERCRED to the current user's uid (the documented
DEV-INTERIM posture). The private key NEVER leaves this process; the sign-*
CLIs are KEYLESS and reach it only over the socket.

This is a long-running process: it serves until it receives SIGINT/SIGTERM,
then removes the socket and exits. Background it (or wire it under launchd)
yourself. It FAILS CLOSED: a missing/unreadable key, a bind failure, or an
empty uid allowlist refuses to start with a non-zero exit and prints nothing
sensitive.

DEV-INTERIM: the signer runs as the SAME uid as the CLI today. This is the
custody ARCHITECTURE, not real isolation — that needs a dedicated service uid +
launchd + sandbox + Secure-Enclave key wrap (deferred).`,
	ConfigPrefix: "app.identitysigner",
	FlagOverrides: map[string]string{
		"app.identitysigner.signer_key":    "signer-key",
		"app.identitysigner.signer_socket": "signer-socket",
	},
}

// IdentitySignerOptions returns config options for `identity signer`.
func IdentitySignerOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysigner.signer_key", DefaultValue: "", Description: "Sealed signer key path (default: XDG data dir)", Type: "string"},
		{Key: "app.identitysigner.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignerOptions)
}
