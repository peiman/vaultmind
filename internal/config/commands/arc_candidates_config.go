package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ArcCandidatesMetadata defines the `arc candidates` command — propose-only
// arc-distillation candidate surfacing (plasticity step 2).
var ArcCandidatesMetadata = config.CommandMetadata{
	Use:   "candidates",
	Short: "Surface arc-distillation candidate moments from episodes (propose-only)",
	Long: "Scan the vault's episode captures and surface candidate transformation moments " +
		"(authority-grants, manifesto-lens invocations) for arc distillation. PROPOSE-ONLY: it " +
		"never writes arcs — the moments are pointers for you to judge, draft, and approve. See " +
		"principle-how-to-write-arcs for the bar a real arc must clear.",
	ConfigPrefix: "app.arc.candidates",
	FlagOverrides: map[string]string{
		"app.arc.candidates.vault": "vault",
		"app.arc.candidates.json":  "json",
	},
}

// ArcCandidatesOptions returns configuration options for `arc candidates`.
func ArcCandidatesOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.arc.candidates.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.arc.candidates.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ArcCandidatesOptions)
}
