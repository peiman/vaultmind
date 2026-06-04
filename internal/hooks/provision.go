package hooks

// ProvisionResult bundles the outcome of installing the hook scripts and
// (optionally) merging the wiring into a project's settings file. It is the
// shared return of Provision, used by both `hooks install --merge` and
// `init --wire-hooks`.
type ProvisionResult struct {
	Install *InstallResult   `json:"install"`
	Merge   *MergeFileResult `json:"merge,omitempty"`
}

// Provision writes the embedded hook scripts into cfg.ProjectDir (Install) and,
// when merge is true and the install hit no conflict, additively merges the
// canonical hook wiring into the project's settings file (MergeIntoSettings).
//
// A script conflict (Install returns an error) gates the merge — we never wire
// settings to point at unresolved scripts — and is returned as the error with
// Merge left nil. This is the single source of truth for the "install + wire"
// sequence behind both `hooks install --merge` and `init --wire-hooks`, so the
// gating and ordering live in exactly one place (manifesto principle 7 — SSOT).
func Provision(cfg InstallConfig, merge, local, dryRun bool) (*ProvisionResult, error) {
	res, err := Install(cfg)
	out := &ProvisionResult{Install: res}
	if !merge || err != nil {
		return out, err
	}
	merged, mErr := MergeIntoSettings(cfg.ProjectDir, cfg.VaultPath, local, dryRun)
	out.Merge = merged
	return out, mErr
}
