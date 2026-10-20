package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func executeNotePinned(cmd *cobra.Command) error {
	paths, err := notePinnedVaults(cmd)
	if err != nil {
		return err
	}
	cfg := memory.PinnedConfig{
		Budget:        getConfigValueWithFlags[int](cmd, "budget", config.KeyAppNotepinnedBudget),
		ExcerptTokens: getConfigValueWithFlags[int](cmd, "excerpt", config.KeyAppNotepinnedExcerpt),
	}
	sources, closer, err := openPinnedVaults(cmd, paths)
	if err != nil {
		return err
	}
	defer closer()

	result, err := memory.PinnedVaults(sources, cfg)
	if err != nil {
		return err
	}
	if result.Total == 0 {
		return nil
	}
	trackPinnedReads(cmd, sources, result)
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNotepinnedJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("note pinned", result))
	}
	return writePinned(cmd.OutOrStdout(), result)
}

// notePinnedVaults is --vaults when given, else --vault. The list parser is
// the one tree, search, and note get use (Principle 7 — one vault list).
func notePinnedVaults(cmd *cobra.Command) ([]string, error) {
	single := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNotepinnedVault)
	list := getConfigValueWithFlags[string](cmd, "vaults", config.KeyAppNotepinnedVaults)
	paths, err := resolveAskVaultPaths(single, list)
	if err != nil {
		return nil, err
	}
	if len(paths) > 1 {
		if err := requireRealVaults(paths); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func openPinnedVaults(cmd *cobra.Command, paths []string) ([]memory.PinnedVault, func(), error) {
	several := len(paths) > 1
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNotepinnedJson)
	var dbs []*cmdutil.VaultDB
	closer := func() {
		for _, db := range dbs {
			db.Close()
		}
	}
	sources := make([]memory.PinnedVault, 0, len(paths))
	for _, p := range paths {
		vdb, err := openOnePinnedVault(cmd, p, several, jsonOut)
		if err != nil {
			closer()
			return nil, nil, err
		}
		dbs = append(dbs, vdb)
		sources = append(sources, memory.PinnedVault{DB: vdb.DB, Vault: p})
	}
	return sources, closer, nil
}

func openOnePinnedVault(cmd *cobra.Command, path string, several, jsonOut bool) (*cmdutil.VaultDB, error) {
	if !several {
		return cmdutil.OpenVaultDBOrWriteErr(cmd, path, "note pinned")
	}
	vdb, err := cmdutil.OpenVaultDB(path)
	if err == nil {
		return vdb, nil
	}
	msg := fmt.Sprintf("note pinned: vault %s: %v", path, err)
	if jsonOut {
		return nil, cmdutil.WriteJSONError(cmd.OutOrStdout(), "note pinned", "vault_error", msg)
	}
	return nil, fmt.Errorf("%s", msg)
}

// trackPinnedReads records the notes whose text was actually included.
// The caller is VAULTMIND_CALLER: the explicit argument is empty so a hook
// value and a workhorse value are both stored as given. Omitted notes were
// not read. Each note is recorded in the vault it came from.
func trackPinnedReads(cmd *cobra.Command, sources []memory.PinnedVault, result *memory.PinnedResult) {
	byVault := make(map[string]*index.DB, len(sources))
	for _, src := range sources {
		byVault[src.Vault] = src.DB
	}
	for _, it := range result.Items {
		db := byVault[it.Vault]
		if db == nil {
			continue
		}
		delivered := it.Excerpt != ""
		if recErr := index.RecordNoteAccessDelivered(db, it.ID, "", delivered); recErr != nil {
			log.Debug().Err(recErr).Str("note_id", it.ID).Msg("note pinned: access record failed")
		}
		if session := experiment.FromContext(cmd.Context()); session != nil {
			session.SetVaultPath(it.Vault)
			_, _ = session.LogNoteAccessEvent(it.ID, experiment.AccessSourceRead, delivered)
		}
	}
}

func writePinned(w io.Writer, r *memory.PinnedResult) error {
	var b strings.Builder
	fmt.Fprintf(&b, "PINNED — %d of %d\n\n", len(r.Items), r.Total)
	for _, it := range r.Items {
		fmt.Fprintf(&b, "%s\n", pinnedHeading(it))
		if it.Excerpt != "" {
			fmt.Fprintf(&b, "%s\n\n", it.Excerpt)
		} else {
			b.WriteString("   (no body)\n\n")
		}
	}
	if len(r.Omitted) > 0 {
		fmt.Fprintf(&b, "── %d pinned note(s) DID NOT FIT the %d-token budget and are NOT above:\n",
			len(r.Omitted), r.Budget)
		for _, n := range r.Omitted {
			fmt.Fprintf(&b, "%s\n", omittedPinnedLine(n))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func pinnedHeading(it memory.PinnedItem) string {
	if it.Vault == "" {
		return fmt.Sprintf("── %s — %s", it.ID, it.Title)
	}
	return fmt.Sprintf("── %s — %s — %s", it.ID, it.Title, it.Vault)
}

func omittedPinnedLine(n memory.OmittedNote) string {
	if n.Vault == "" {
		return fmt.Sprintf("   %s — %s", n.ID, n.Title)
	}
	return fmt.Sprintf("   %s — %s — %s", n.ID, n.Title, n.Vault)
}
