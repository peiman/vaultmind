package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/cmdutil"
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

// requireRealVaults rejects any listed path that is not already a vault,
// BEFORE anything is opened.
//
// The single-vault rule deliberately lets a NAMED path be created: you said
// --vault X, so X becomes a vault. That reasoning does not survive a list.
// Observed live: `--vaults real,typo` created `typo/.vaultmind/index.db` on
// the way past and then reported the directory as searched — "unmeasured, no
// embedder, so kept in the merge". With one vault an empty answer is plainly
// about the path you typed; with three, the others answer and the typo
// vanishes into a header claiming full coverage. A federation that reports
// having searched somewhere it never looked is the exact failure this feature
// exists to end, so a mistyped path is an error, not a new vault.
func requireRealVaults(paths []string) error {
	var bad []string
	for _, p := range paths {
		if !cmdutil.IsVaultRoot(p) {
			bad = append(bad, p)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	subject, verb := "paths that are not vaults", "them"
	if len(bad) == 1 {
		subject, verb = "path that is not a vault", "it"
	}
	return fmt.Errorf(
		"--vaults lists %d %s: %s\n"+
			"  Federation will not create %s: a mistyped path would be silently counted as searched.\n"+
			"  Create one with:  vaultmind init <path>",
		len(bad), subject, strings.Join(bad, ", "), verb)
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
	if err := requireRealVaults(paths); err != nil {
		return nil, nil, "", err
	}
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

	// ONE retrieval, then judge those same hits. Searching again for the
	// verdict doubled every federated query — 6.7s across three vaults, enough
	// to push the recall hook past its timeout and make it inject nothing.
	return query.SearchAndJudge(
		cmd.Context(), ret.Retriever, ret.Embedder, vdb.DB,
		vaultFloorResolver(cmd, vaultPath), queryText, searchLimit)
}

// vaultFloorResolver binds a vault's calibration lookup for SearchAndJudge, so
// a federated judgement and a direct `ask --vault X` cannot disagree about
// whether X has anything relevant.
func vaultFloorResolver(cmd *cobra.Command, vaultPath string) query.FloorResolver {
	return func(dims int) (float64, float64) {
		floor, sigma, _ := resolveNoiseFloor(cmd.Context(), vaultPath, dims)
		return floor, sigma
	}
}
