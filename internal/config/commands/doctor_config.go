package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DoctorMetadata defines the metadata for the doctor command.
var DoctorMetadata = config.CommandMetadata{
	Use:          "doctor",
	Short:        "Vault health summary",
	Long:         "Run diagnostics on the vault index and report issues.",
	ConfigPrefix: "app.doctor",
	FlagOverrides: map[string]string{
		"app.doctor.vault": "vault",
		"app.doctor.json":  "json",
	},
}

// DoctorOptions returns configuration options for the doctor command.
func DoctorOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctor.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctor.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorOptions)
}
