package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// evalEnvelope names the command in its JSON envelope.
const evalEnvelope = "eval"

// evalMinLimit is how many results each query fetches at least, so a relevant
// note ranked past K is still reported with its rank rather than as missing.
const evalMinLimit = 10

// evalQuery is one query's outcome. Rank is the 1-based rank of the first
// relevant note, 0 when none was in the fetched results.
type evalQuery struct {
	Name     string   `json:"name"`
	Text     string   `json:"text"`
	Expected []string `json:"expected"`
	Rank     int      `json:"rank"`
	Results  []string `json:"results"`
}

// evalResult is the scored run.
type evalResult struct {
	Vault      string      `json:"vault"`
	Retrieval  string      `json:"retrieval"`
	K          int         `json:"k"`
	QueryCount int         `json:"query_count"`
	HitAt1     float64     `json:"hit_at_1"`
	HitAtK     float64     `json:"hit_at_k"`
	MRR        float64     `json:"mrr"`
	Queries    []evalQuery `json:"queries"`
	limit      int
}

// evaluateVault runs the labelled queries in path against the vault's own
// retriever — the same one ask uses — and scores them.
func evaluateVault(cmd *cobra.Command, path string) (*evalResult, error) {
	queries, err := baseline.LoadQueries(path)
	if err != nil {
		return nil, fmt.Errorf("eval: %w", err)
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppEvalVault)
	k := getConfigValueWithFlags[int](cmd, "k", config.KeyAppEvalK)
	if k < 1 {
		return nil, fmt.Errorf("eval: --k must be at least 1, got %d", k)
	}
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("eval: %w", err)
	}
	defer vdb.Close()
	ret := query.BuildAutoRetrieverFull(cmd.Context(), vdb.DB)
	defer ret.Cleanup()

	limit := max(k, evalMinLimit)
	rep, err := baseline.Run(ret.Retriever, queries, baseline.RunConfig{K: k, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("eval: %w", err)
	}
	return scoreEval(rep, vaultPath, retrievalModeLabel(ret), limit), nil
}

// scoreEval turns the baseline report into the command's result.
func scoreEval(rep *baseline.Report, vaultPath, mode string, limit int) *evalResult {
	res := &evalResult{Vault: vaultPath, Retrieval: mode, K: rep.K, QueryCount: len(rep.Queries),
		HitAtK: rep.HitAtK, MRR: rep.MRR, limit: limit}
	hits1 := 0
	for _, q := range rep.Queries {
		rank := 0
		if q.ReciprocalRank > 0 {
			rank = int(1/q.ReciprocalRank + 0.5)
		}
		if rank == 1 {
			hits1++
		}
		res.Queries = append(res.Queries, evalQuery{Name: q.Name, Text: q.Text, Expected: q.Expected, Rank: rank, Results: q.ResultIDs})
	}
	res.HitAt1 = float64(hits1) / float64(len(rep.Queries))
	return res
}

func writeEval(cmd *cobra.Command, res *evalResult) error {
	w := cmd.OutOrStdout()
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppEvalJson) {
		return json.NewEncoder(w).Encode(envelope.OK(evalEnvelope, res))
	}
	return writeEvalText(w, res)
}

func writeEvalText(w io.Writer, res *evalResult) error {
	n := res.QueryCount
	lines := []string{
		fmt.Sprintf("Eval: %d queries against %s (%s)", n, res.Vault, res.Retrieval),
		fmt.Sprintf("  Hit@1   %2d/%d  (%.2f)", int(res.HitAt1*float64(n)+0.5), n, res.HitAt1),
		fmt.Sprintf("  Hit@%-2d  %2d/%d  (%.2f)", res.K, int(res.HitAtK*float64(n)+0.5), n, res.HitAtK),
		fmt.Sprintf("  MRR     %.3f", res.MRR),
	}
	width := 0
	for _, q := range res.Queries {
		width = max(width, len(q.Name))
	}
	var missed []string
	for _, q := range res.Queries {
		if q.Rank == 1 {
			continue
		}
		where := fmt.Sprintf("rank %d", q.Rank)
		if q.Rank == 0 {
			where = fmt.Sprintf("not in the top %d", res.limit)
		}
		missed = append(missed, fmt.Sprintf("  %-*s  %-17s %q", width, q.Name, where, q.Text))
	}
	if len(missed) > 0 {
		lines = append(lines, "", "Not answered first:")
		lines = append(lines, missed...)
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}
