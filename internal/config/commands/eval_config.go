package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// EvalMetadata defines the `eval` command — retrieval scored against labels.
var EvalMetadata = config.CommandMetadata{
	Use:   "eval <queries.yaml>",
	Short: "Score a vault's retrieval against labelled queries: Hit@1, Hit@K, MRR",
	Long: "Run each labelled query through the vault's retriever and report how often a " +
		"relevant note came first (Hit@1), made the top K (Hit@K), and the mean reciprocal " +
		"rank of the first relevant note (MRR). Queries that did not put a relevant note " +
		"first are listed with the rank they got.\n\n" +
		"The query file is a YAML list, one entry per query:\n\n" +
		"  - name: rrf\n" +
		"    text: how does reciprocal rank fusion combine lanes\n" +
		"    expected: [concept-reciprocal-rank-fusion, concept-hybrid-search]\n\n" +
		"`expected` names the notes a good answer surfaces; order is not significant. Use " +
		"real questions and label them without looking at the current ranking, so a " +
		"change to retrieval can be judged better or worse, not just different.\n\n" +
		"  vaultmind eval queries.yaml --vault ./vaultmind-vault\n" +
		"  vaultmind eval queries.yaml --vault ./vaultmind-vault --k 10 --json",
	ConfigPrefix: "app.eval",
	FlagOverrides: map[string]string{
		"app.eval.vault": "vault",
		"app.eval.k":     "k",
		"app.eval.json":  "json",
	},
}

// EvalOptions returns configuration options for `eval`.
func EvalOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.eval.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.eval.k", DefaultValue: 5, Description: "K for Hit@K: a query counts as a hit when a relevant note is in the top K", Type: "int"},
		{Key: "app.eval.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(EvalOptions)
}
