package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ArcCandidatesMetadata defines the `arc candidates` command — propose-only
// arc-distillation candidate surfacing (plasticity step 2).
var ArcCandidatesMetadata = config.CommandMetadata{
	Use:   "candidates",
	Short: "Surface arc-distillation candidate moments from episodes and the desk (propose-only)",
	Long: "Scan the vault and surface candidate transformation moments for arc distillation. " +
		"PROPOSE-ONLY: it never writes arcs — the moments are pointers for you to judge, draft, " +
		"and approve. See principle-how-to-write-arcs for the bar a real arc must clear.\n\n" +
		"Two sources are scanned:\n" +
		"  <vault>/episodes  session captures, phrase-matched for candidate moments\n" +
		"                    (authority-grants, manifesto-lens invocations) — guesses to check.\n" +
		"  the desk          any note in the vault whose frontmatter says `type: journal`: raw\n" +
		"                    entries written mid-session, already judged worth keeping. An entry\n" +
		"                    carrying `distilled_to: <arc-id>` is finished, and is not surfaced\n" +
		"                    again.\n\n" +
		"Each proposal is listed with the existing arcs it most resembles, so a new shape is " +
		"distinguishable from one already written. That comparison needs embeddings: run " +
		"`vaultmind index --embed` to enable it. Without them the proposals still appear — only " +
		"the neighbours are missing, and the report says so.",
	ConfigPrefix: "app.arc.candidates",
	FlagOverrides: map[string]string{
		"app.arc.candidates.vault":      "vault",
		"app.arc.candidates.json":       "json",
		"app.arc.candidates.arcs_vault": "arcs-vault",
	},
}

// ArcCandidatesOptions returns configuration options for `arc candidates`.
func ArcCandidatesOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.arc.candidates.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.arc.candidates.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.arc.candidates.arcs_vault", DefaultValue: "", Description: "Vault holding the existing arcs to compare proposals against (default: the scanned vault). Set this when the desk and the arcs live in different vaults", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ArcCandidatesOptions)
}
