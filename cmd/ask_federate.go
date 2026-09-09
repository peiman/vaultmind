package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/spf13/cobra"
)

// Federated ask — search several vaults, deliver from the one that owns the
// answer.
//
// An agent's memory is not one vault. Mine is three, every recall hook
// searched one, and the measured cost was 1-of-4 coverage with two of the
// misses already written down in the desk. `--vaults` closes that.
//
// Delivery deliberately stays SINGLE-vault: the merge picks an owner, and the
// owner's own pipeline packs the body, applies its own noise floor, and
// records the access. There is no merged pseudo-vault whose floor belongs to
// nobody, and plasticity lands in the vault the note actually lives in.

// resolveAskVaultPaths returns the vaults a run will search: just the single
// --vault when --vaults is empty, else the parsed list. Blank entries are
// dropped and duplicates removed — the same vault listed twice would score
// twice in the RRF merge and silently outrank the others.
func resolveAskVaultPaths(single, vaults string) ([]string, error) {
	if strings.TrimSpace(vaults) == "" {
		return []string{single}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, 3)
	for _, raw := range strings.Split(vaults, ",") {
		p := strings.TrimSpace(raw)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf(
			"--vaults %q lists no usable vault path; asking to federate across nothing must not silently become a single-vault search", vaults)
	}
	return out, nil
}

// vaultDisplayName is the directory name, which is what an operator recognises
// ("vaultmind-mine"), used to tag every hit with its origin.
func vaultDisplayName(path string) string {
	return filepath.Base(filepath.Clean(path))
}

// federateAndPickOwner searches every vault and returns the merged ranked list
// plus the path of the vault owning the top hit.
//
// A vault that fails to open is reported, never skipped: a federation that
// quietly drops a vault answers "nothing relevant" while not having looked,
// which is the failure this whole feature exists to end.
func federateAndPickOwner(cmd *cobra.Command, queryText string, paths []string, searchLimit int) ([]query.FederatedHit, []query.FederatedVaultStatus, string, error) {
	perVault := make(map[string][]retrieval.ScoredResult, len(paths))
	verdicts := make(map[string]string, len(paths))
	relevance := make(map[string]float64, len(paths))
	byName := make(map[string]string, len(paths))
	names := make([]string, 0, len(paths))

	// ONE VAULT AT A TIME, opened and released before the next.
	//
	// The ORT/hugot embedder allows a single session per process. Holding all
	// vaults open at once gave the first one the embedder and silently dropped
	// the rest to KEYWORD search — while reporting them as "unmeasured, no
	// embedder", so a vault with a full BGE-M3 index looked like one without.
	// Found only by running the real ORT binary; the earlier measurement was
	// taken on a test build with no such limit and described a binary nobody
	// runs.
	for _, p := range paths {
		name := vaultDisplayName(p)
		byName[name] = p
		names = append(names, name)

		hits, verdict, z, err := searchOneVault(cmd, p, queryText, searchLimit)
		if err != nil {
			return nil, nil, "", fmt.Errorf("federated ask: vault %s: %w", p, err)
		}
		perVault[name] = hits
		verdicts[name] = verdict
		relevance[name] = z
	}

	merged := query.MergeFederated(perVault, verdicts, relevance)

	contributed := map[string]bool{}
	for _, h := range merged {
		contributed[h.Vault] = true
	}
	statuses := make([]query.FederatedVaultStatus, 0, len(names))
	for _, n := range names {
		statuses = append(statuses, query.FederatedVaultStatus{
			Name: n, Verdict: verdicts[n], Contributed: contributed[n],
		})
	}

	if len(merged) == 0 {
		// Nothing anywhere. Fall back to the primary vault so the caller still
		// gets the normal no-match path, floor and diagnostics.
		return nil, statuses, paths[0], nil
	}
	owner, ok := byName[merged[0].Vault]
	if !ok {
		return nil, nil, "", fmt.Errorf("federated ask: merged top hit names unknown vault %q", merged[0].Vault)
	}
	return merged, statuses, owner, nil
}

// searchOneVault opens a vault, searches it, computes its OWN relevance
// verdict, and closes everything before returning — so the next vault can
// claim the single available embedder session.
func searchOneVault(cmd *cobra.Command, vaultPath, queryText string, searchLimit int) ([]retrieval.ScoredResult, string, float64, error) {
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return nil, "", 0, err
	}
	defer vdb.Close()

	ret := query.BuildAutoRetrieverFull(vdb.DB)
	defer ret.Cleanup()

	res, err := query.AskHits(cmd.Context(), ret.Retriever, queryText, searchLimit)
	if err != nil {
		return nil, "", 0, err
	}
	verdict, z := vaultOwnVerdict(cmd, vaultPath, queryText, ret, vdb, searchLimit)
	if res == nil {
		return nil, verdict, z, nil
	}
	return res.TopHits, verdict, z, nil
}

// vaultOwnVerdict computes one vault's top-hit confidence using that vault's
// own noise floor — the same derivation the single-vault path uses, so a
// federated judgement and a direct `ask --vault X` cannot disagree about
// whether X has anything relevant.
//
// Returns "" when the vault has no embedder (keyword-only): unmeasured, which
// the gate keeps rather than treating as irrelevant.
func vaultOwnVerdict(cmd *cobra.Command, vaultPath, queryText string, ret query.AutoRetrieverResult, vdb *cmdutil.VaultDB, searchLimit int) (string, float64) {
	if ret.Embedder == nil {
		return "", 0
	}
	ctx := cmd.Context()
	hits, err := query.AskHits(ctx, ret.Retriever, queryText, searchLimit)
	if err != nil || hits == nil || len(hits.TopHits) == 0 {
		return "", 0
	}
	sims, err := query.NoteSimilarities(ctx, queryText, ret.Embedder, vdb.DB)
	if err != nil || sims == nil {
		return "", 0
	}
	topCosine, ok := sims[hits.TopHits[0].ID]
	if !ok {
		return "", 0
	}
	floor, sigma, _ := resolveNoiseFloor(ctx, vaultPath, ret.Embedder.Dims())
	z, label := noisefloor.Relevance(topCosine, floor, sigma, noisefloor.DefaultNoiseFloor(ret.Embedder.Dims()))
	return label, z
}
