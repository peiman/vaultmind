package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityFetchRegistryMetadata defines metadata for the
// `identity fetch-registry` command.
var IdentityFetchRegistryMetadata = config.CommandMetadata{
	Use:   "fetch-registry",
	Short: "Fetch the signed trust registry from the hub and keep the local copy current (Contract-B)",
	Long: `Download the signed registry the hub serves at /.well-known/vaultmind-directory,
VERIFY it against the pinned root key, and store it at this machine's declared
registry_path — so doctor and the mesh watcher can check it and count down to
its lapse without anyone copying files by hand.

Trust comes only from the root signature, never from the connection:
  - it refuses to run without a pinned root (--root-pubkey or its config key
    app.identityfetchregistry.root_pubkey, else the anchor
    ` + "`identity enroll`" + ` pinned);
  - it refuses anything that does not verify, or is past its valid_until;
  - it refuses an OLDER epoch than the local copy (a rollback);
  - the same epoch writes nothing; a newer one replaces the file atomically.

The hub address and registry path come from --hub / --registry-file, else from
agents.yaml (daemon_url, registry_path). Nothing is guessed: an undeclared
address or path is an error.`,
	ConfigPrefix: "app.identityfetchregistry",
	FlagOverrides: map[string]string{
		"app.identityfetchregistry.hub":           "hub",
		"app.identityfetchregistry.registry_file": "registry-file",
		"app.identityfetchregistry.root_pubkey":   "root-pubkey",
	},
}

// IdentityFetchRegistryOptions returns config options for `identity fetch-registry`.
func IdentityFetchRegistryOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identityfetchregistry.hub", DefaultValue: "", Description: "Hub base URL (default: AGENT_CHAT_DAEMON_URL, else agents.yaml daemon_url)", Type: "string"},
		{Key: "app.identityfetchregistry.registry_file", DefaultValue: "", Description: "Where to store the registry (default: agents.yaml registry_path)", Type: "string"},
		{Key: "app.identityfetchregistry.root_pubkey", DefaultValue: "", Description: "Pinned root public key, base64 (default: the anchor identity enroll pinned)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityFetchRegistryOptions)
}
