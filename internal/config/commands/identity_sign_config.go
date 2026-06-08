package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignMetadata defines metadata for the `identity sign` command.
var IdentitySignMetadata = config.CommandMetadata{
	Use:   "sign",
	Short: "Validate, canonicalize, and sign an entry via the keyless signer",
	Long: `Read a Contract-B entry (stdin or --file), VALIDATE and CANONICALIZE it
(slice-1 schema gate + RFC 8785 JCS), then send the canonical bytes to the
signer over its 0600 socket and print the signature.

This CLI is KEYLESS: it NEVER opens the private-key file. If the signer is
unreachable it FAILS CLOSED with an error (never a silent unsigned result).`,
	ConfigPrefix: "app.identitysign",
	FlagOverrides: map[string]string{
		"app.identitysign.file":          "file",
		"app.identitysign.signer_socket": "signer-socket",
	},
}

// IdentitySignOptions returns config options for `identity sign`.
func IdentitySignOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysign.file", DefaultValue: "", Description: "Read entry JSON from this file instead of stdin", Type: "string"},
		{Key: "app.identitysign.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignOptions)
}
