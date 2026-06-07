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
--all composes with --summary and --json (one combined envelope).`,
	ConfigPrefix: "app.doctor",
	FlagOverrides: map[string]string{
		"app.doctor.vault":   "vault",
		"app.doctor.json":    "json",
		"app.doctor.summary": "summary",
		"app.doctor.all":     "all",
		"app.doctor.root":    "root",
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
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorOptions)
}
