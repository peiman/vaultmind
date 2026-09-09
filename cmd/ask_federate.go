package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
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
	sources := make([]query.VaultSource, 0, len(paths))
	byName := make(map[string]string, len(paths))

	// One cleanup for all handles rather than a defer per iteration: a
	// deferred close inside a loop holds every vault's DB open until the
	// function returns, which is harmless at three vaults and a leak at thirty.
	var closers []func()
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}()

	for _, p := range paths {
		vdb, err := cmdutil.OpenVaultDB(p)
		if err != nil {
			return nil, nil, "", fmt.Errorf("federated ask: opening vault %s: %w", p, err)
		}
		closers = append(closers, vdb.Close)

		ret := query.BuildAutoRetrieverFull(vdb.DB)
		closers = append(closers, ret.Cleanup)

		name := vaultDisplayName(p)
		byName[name] = p

		// Each vault judges its OWN relevance against its OWN floor. Without
		// this the merge is relevance-blind: every vault's rank-1 earns equal
		// credit, so a vault holding nothing relevant still wins with its best
		// irrelevant note (observed live: a z=-1.25 no_match note outranking
		// the desk entry that answered the question).
		verdict, z := vaultOwnVerdict(cmd, p, queryText, ret, vdb, searchLimit)
		sources = append(sources, query.VaultSource{
			Name: name, Retriever: ret.Retriever, Verdict: verdict, RelevanceZ: z,
		})
	}

	merged, err := query.FederatedSearch(context.WithoutCancel(cmd.Context()), sources, queryText, searchLimit)
	if err != nil {
		// Partial failures come back WITH results; surface the error either way
		// rather than presenting a partial search as a complete one.
		if len(merged) == 0 {
			return nil, nil, "", err
		}
		if _, werr := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err); werr != nil {
			return nil, nil, "", werr
		}
	}
	// Status for EVERY vault searched, contributing or not.
	contributed := map[string]bool{}
	for _, h := range merged {
		contributed[h.Vault] = true
	}
	statuses := make([]query.FederatedVaultStatus, 0, len(sources))
	for _, src := range sources {
		statuses = append(statuses, query.FederatedVaultStatus{
			Name: src.Name, Verdict: src.Verdict, Contributed: contributed[src.Name],
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
