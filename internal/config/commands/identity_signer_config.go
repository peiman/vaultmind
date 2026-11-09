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
then removes the socket and exits. Nothing restarts it if it dies: run
` + "`vaultmind identity signer install`" + ` to put it under launchd (starts at login,
restarts on exit). It FAILS CLOSED: a missing/unreadable key, a bind failure, or an
empty uid allowlist refuses to start with a non-zero exit and prints nothing
sensitive.

ONE SIGNATURE ONLY (--once): sign exactly one request, then close the socket,
print to stderr

    signed once: sha256=<lowercase hex> bytes=<n>

and exit 0. The hash is over exactly the bytes that were signed. The signer
signs with pure Ed25519 and adds no prefix or domain separation, so these are
the bytes the client sent: for sign-envelope, the envelope's JCS canonical
bytes. A verifier recomputes sha256 over those canonical bytes and compares.
Because only one signature can exist, a matching hash proves the signature
made is over the bytes you checked. Requests that arrive while the first is
being signed, or after it, are refused. Failed requests (wrong uid, malformed
frame, policy refusal) produce no signature and do not count, but after 3 of
them the signer gives up, signs nothing, and exits non-zero. A connection that
closes without sending anything (doctor's liveness probe) is not a failure. Stopped by
SIGINT/SIGTERM before a signature, it exits 0 and prints no hash.

DEV-INTERIM: the signer runs as the SAME uid as the CLI today. This is the
custody ARCHITECTURE, not real isolation — that needs a dedicated service uid +
launchd + sandbox + Secure-Enclave key wrap (deferred).`,
	ConfigPrefix: "app.identitysigner",
	FlagOverrides: map[string]string{
		"app.identitysigner.signer_key":    "signer-key",
		"app.identitysigner.signer_socket": "signer-socket",
		"app.identitysigner.once":          "once",
	},
}

// IdentitySignerOptions returns config options for `identity signer`.
func IdentitySignerOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysigner.signer_key", DefaultValue: "", Description: "Sealed signer key path (default: XDG data dir)", Type: "string"},
		{Key: "app.identitysigner.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
		{Key: "app.identitysigner.once", DefaultValue: false, Description: "Sign exactly one request, print its sha256 to stderr, then exit", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignerOptions)
}
