package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignerInstallMetadata defines `identity signer install` — run the
// custody signer under launchd so it survives logouts, reboots and crashes.
//
// Without it the signer is a foreground process that nothing restarts. On
// 2026-09-22 a member's signer died with a session reset and every signed send
// failed "signer unreachable" until someone restarted it by hand.
var IdentitySignerInstallMetadata = config.CommandMetadata{
	Use:   "install",
	Short: "Run the signer under launchd: start at login, restart if it dies (macOS)",
	Long: "Write a LaunchAgent for the custody signer and load it, so the signer starts at " +
		"login and is restarted whenever it exits. Without this the signer is a foreground " +
		"process nothing restarts: when it dies, every signed send fails \"signer unreachable\" " +
		"until someone notices.\n\n" +
		"The job launches exactly the signer you would start by hand. The binary and the " +
		"key/socket paths are resolved NOW, with your flags and --config-path-mode, and written " +
		"into the job as absolute paths — launchd runs with its own environment, and a default " +
		"resolved there is not the one your shell resolves.\n\n" +
		"Refuses while another signer is already serving the socket: the launchd copy would fail " +
		"to bind and KeepAlive would turn that into a restart loop that looks like a running " +
		"service. Stop the hand-started signer first.\n\n" +
		"Rerunning replaces the job. One job per signer name (default: the key file's name), so " +
		"two signers on one machine get two jobs.\n\n" +
		"EXAMPLES\n\n" +
		"  vaultmind identity signer install --print\n" +
		"      Show the job without installing anything — for review before it touches a machine.\n\n" +
		"  vaultmind identity signer install --config-path-mode native\n" +
		"      Install for a signer whose key lives under the native (Application Support) paths.\n\n" +
		"  vaultmind identity signer install --signer-key ~/.config/vaultmind/contractb/mira.key\n" +
		"      Install for an explicit key; the job is named after it (com.vaultmind.signer.mira).",
	ConfigPrefix: "app.identitysignerinstall",
	FlagOverrides: map[string]string{
		"app.identitysignerinstall.signer_key":    "signer-key",
		"app.identitysignerinstall.signer_socket": "signer-socket",
		"app.identitysignerinstall.name":          "name",
		"app.identitysignerinstall.print":         "print",
	},
}

// IdentitySignerInstallOptions returns config options for `identity signer install`.
func IdentitySignerInstallOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignerinstall.signer_key", DefaultValue: "", Description: "Sealed signer key path (default: XDG data dir, per --config-path-mode)", Type: "string"},
		{Key: "app.identitysignerinstall.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir, per --config-path-mode)", Type: "string"},
		{Key: "app.identitysignerinstall.name", DefaultValue: "", Description: "Job name suffix (default: the key file's name without extension)", Type: "string"},
		{Key: "app.identitysignerinstall.print", DefaultValue: false, Description: "Print the LaunchAgent plist and exit without writing or loading anything", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignerInstallOptions)
}
