package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DoctorMetadata defines the metadata for the doctor command.
var DoctorMetadata = config.CommandMetadata{
	Use:   "doctor",
	Short: "Vault health hub: diagnose the vault and report issues",
	Long: `Run read-only diagnostics on the vault and report its health.

doctor is the single vault-health command. It reports note counts, the per-type
breakdown (per-type counts, required fields, valid statuses), embedding
readiness, hook drift, unresolved/incompatible/dead links, stale-index drift,
and an errors/warnings rollup.

By default doctor also prints per-link details, which can run to hundreds of
lines on a noisy vault. Use --summary for the cold-start view: counts, per-type
breakdown, and the errors/warnings rollup, with the verbose per-link detail
lines suppressed. (--summary replaces the former 'vault status'.)

To repair what doctor finds, run 'doctor heal' (all auto-fixable) or
'doctor heal wikilinks'.

Use --all to diagnose EVERY vault discovered under a root (directories
containing a .vaultmind/ subdir), with a combined rollup plus a per-vault
report. --root sets where discovery starts (default: the current directory).
--all composes with --summary and --json (one combined envelope).

When a Contract-B mesh signal exists (an identity key, a pinned network anchor,
a --mesh-* flag, or a reachable local daemon) doctor also reports a mesh-identity
section: key custody, whether your binding authenticates against a PINNED root,
and chat reachability. Green is reserved for a cryptographically authenticated
binding; everything else is a warning (doctor stays exit-0). --mesh-root-pubkey
pins the root explicitly (otherwise the enroll-persisted anchor is used),
--mesh-registry verifies an offline registry file, --mesh-slug overrides the
agents.yaml slug, and --mesh-heartbeat overrides the wake-watcher heartbeat path.`,
	ConfigPrefix: "app.doctor",
	FlagOverrides: map[string]string{
		"app.doctor.vault":            "vault",
		"app.doctor.json":             "json",
		"app.doctor.summary":          "summary",
		"app.doctor.all":              "all",
		"app.doctor.root":             "root",
		"app.doctor.mesh_root_pubkey": "mesh-root-pubkey",
		"app.doctor.mesh_registry":    "mesh-registry",
		"app.doctor.mesh_slug":        "mesh-slug",
		"app.doctor.mesh_heartbeat":   "mesh-heartbeat",
	},
}

// DoctorOptions returns configuration options for the doctor command.
func DoctorOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctor.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctor.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.doctor.summary", DefaultValue: false, Description: "Print summary counts only (suppress per-link details)", Type: "bool"},
		{Key: "app.doctor.all", DefaultValue: false, Description: "Diagnose every vault discovered under --root (multi-vault health)", Type: "bool"},
		{Key: "app.doctor.root", DefaultValue: ".", Description: "Root directory to discover vaults under when --all is set", Type: "string"},
		{Key: "app.doctor.mesh_root_pubkey", DefaultValue: "", Description: "Pin the Contract-B network root pubkey (base64) for authenticated mesh health; overrides the enroll-persisted anchor", Type: "string"},
		{Key: "app.doctor.mesh_registry", DefaultValue: "", Description: "Verify an offline Contract-B signed-registry file instead of fetching from the local daemon", Type: "string"},
		{Key: "app.doctor.mesh_slug", DefaultValue: "", Description: "Override the agent slug used to resolve your binding in the mesh registry (default: agents.yaml)", Type: "string"},
		{Key: "app.doctor.mesh_heartbeat", DefaultValue: "", Description: "Override the wake-watcher heartbeat file path used for mesh watcher-liveness", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorOptions)
}
