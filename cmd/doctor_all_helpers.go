package cmd

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/discovery"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// runDoctorAll discovers every vault under root and reports a combined health
// view: a workspace rollup followed by each vault's full doctor report. In JSON
// mode it emits ONE combined envelope (result.rollup + result.vaults +
// result.failed), never one envelope per vault. A vault that fails to open or
// diagnose is SURFACED — named with its reason under result.failed[] (JSON) or a
// "Vaults that failed to open" section (human), warned to stderr, and counted in
// the honest discovered/diagnosed/failed breakdown — never silently dropped.
// summaryOnly composes through to each vault's body.
//
// A discovery failure (e.g. a missing root) is reported as a JSON error
// envelope under --json, or a plain command error otherwise. Zero discovered
// vaults is NOT an error: the user gets a clear "no vaults" message (human) or
// an empty rollup envelope (JSON).
func runDoctorAll(cmd *cobra.Command, root string, jsonOut, summaryOnly bool) error {
	paths, err := discovery.DiscoverVaults(root, discovery.DefaultMaxDepth)
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "doctor", "discovery_failed", err.Error())
		}
		return fmt.Errorf("discovering vaults under %q: %w", root, err)
	}

	results, failed := diagnoseAll(cmd, paths)
	all := query.DoctorAllResult{
		Rollup: query.BuildDoctorRollup(results, failed),
		Vaults: results,
		Failed: failed,
	}

	if jsonOut {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), "doctor", all, root, "")
	}
	return writeDoctorAllHuman(cmd.OutOrStdout(), root, all, summaryOnly)
}

// diagnoseAll diagnoses each discovered vault, preserving discovery order
// (already sorted). A vault that fails to open or diagnose does NOT abort the
// whole run — one corrupt vault must not blind the operator to the rest — but it
// is also never silently dropped: each failure is captured (path + reason) in
// the returned slice AND warned to stderr, so the broken vault can never become
// invisible. Per-vault opens use the plain OpenVaultDB so a failure never writes
// a stray JSON error envelope (which would break the single-envelope contract).
func diagnoseAll(cmd *cobra.Command, paths []string) ([]*query.DoctorResult, []query.FailedVault) {
	results := make([]*query.DoctorResult, 0, len(paths))
	var failed []query.FailedVault
	fail := func(p string, err error) {
		failed = append(failed, query.FailedVault{VaultPath: p, Error: err.Error()})
		// Best-effort warning; a failed stderr write must not mask the diagnosis.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping vault %s: %v\n", p, err)
	}
	for _, p := range paths {
		vdb, err := cmdutil.OpenVaultDB(p)
		if err != nil {
			fail(p, err)
			continue
		}
		result, derr := populateDoctorResult(vdb, p)
		vdb.Close()
		if derr != nil {
			fail(p, derr)
			continue
		}
		results = append(results, result)
	}
	return results, failed
}

// writeDoctorAllHuman renders the combined rollup header then each vault's full
// doctor body under a per-vault separator. Zero discovered vaults yields a
// single clear line so the operator never sees empty output.
func writeDoctorAllHuman(w io.Writer, root string, all query.DoctorAllResult, summaryOnly bool) error {
	if err := writeRollupHeader(w, root, all.Rollup); err != nil {
		return err
	}
	if all.Rollup.Discovered == 0 {
		_, err := fmt.Fprintf(w, "No vaults found under %s (looked for directories containing .vaultmind/).\n", root)
		return err
	}
	for _, v := range all.Vaults {
		if _, err := fmt.Fprintf(w, "\n=== %s ===\n", v.VaultPath); err != nil {
			return err
		}
		if err := writeDoctorHuman(w, v, summaryOnly); err != nil {
			return err
		}
	}
	return writeFailedVaults(w, all.Failed)
}

// writeFailedVaults renders a clearly-visible section naming every vault that
// could not be opened, with its reason — so a corrupt vault is surfaced to the
// operator rather than vanishing. Prints nothing when no vault failed.
func writeFailedVaults(w io.Writer, failed []query.FailedVault) error {
	if len(failed) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nVaults that failed to open: %d\n", len(failed)); err != nil {
		return err
	}
	for _, f := range failed {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", f.VaultPath, f.Error); err != nil {
			return err
		}
	}
	return nil
}

// writeRollupHeader prints the workspace-level summary that leads --all output:
// vault count, total notes, combined errors/warnings, and which vaults have
// issues. Printed before any per-vault detail so the operator scans the bottom
// line first.
func writeRollupHeader(w io.Writer, root string, r query.DoctorRollup) error {
	// "Discovered N vault(s)" always reflects the total found. When any vault
	// failed to open, append the honest "(M diagnosed, K failed)" breakdown so
	// the count never hides a broken vault behind a lower diagnosed-only number.
	breakdown := ""
	if r.Failed > 0 {
		breakdown = fmt.Sprintf(" (%d diagnosed, %d failed)", r.Diagnosed, r.Failed)
	}
	if _, err := fmt.Fprintf(w,
		"Doctor --all under %s\nDiscovered %d vault(s)%s\nTotal notes: %d\nTotal issues: %d errors, %d warnings\n",
		root, r.Discovered, breakdown, r.TotalNotes, r.TotalErrors, r.TotalWarnings); err != nil {
		return err
	}
	if len(r.VaultsWithIssues) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Vaults with issues: %d\n", len(r.VaultsWithIssues)); err != nil {
		return err
	}
	for _, p := range r.VaultsWithIssues {
		if _, err := fmt.Fprintf(w, "  %s\n", p); err != nil {
			return err
		}
	}
	return nil
}
