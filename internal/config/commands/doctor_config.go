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
lines suppressed. (--summary replaces the former 'vault status'.)`,
	ConfigPrefix: "app.doctor",
	FlagOverrides: map[string]string{
		"app.doctor.vault":   "vault",
		"app.doctor.json":    "json",
		"app.doctor.summary": "summary",
	},
}

// DoctorOptions returns configuration options for the doctor command.
func DoctorOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctor.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctor.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.doctor.summary", DefaultValue: false, Description: "Print summary counts only (suppress per-link details)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorOptions)
}
