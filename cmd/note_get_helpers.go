package cmd

import (
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/spf13/cobra"
)

// noteGetVault returns the vault to read from: --vault, or with --vaults the
// one vault that holds the note. A federated ask ranks ids from every vault
// but names only the delivering one, so an id copied from the ranking needs a
// lookup that searches them all. An id held by more than one vault is refused
// rather than picked: choosing silently is how `--vault A --vault B` came to
// answer from B alone.
func noteGetVault(cmd *cobra.Command, input string) (string, error) {
	single := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteVault)
	paths, err := resolveAskVaultPaths(single,
		getConfigValueWithFlags[string](cmd, "vaults", config.KeyAppNoteVaults))
	if err != nil {
		return "", err
	}
	if len(paths) == 1 {
		return paths[0], nil
	}
	if err := requireRealVaults(paths); err != nil {
		return "", err
	}
	holders, err := vaultsHolding(input, paths)
	if err != nil {
		return "", err
	}
	if len(holders) == 1 {
		return holders[0], nil
	}
	return "", noteGetVaultError(cmd, input, paths, holders)
}

// vaultsHolding returns the vaults in which input resolves to a note.
func vaultsHolding(input string, paths []string) ([]string, error) {
	var holders []string
	for _, p := range paths {
		vdb, err := cmdutil.OpenVaultDB(p)
		if err != nil {
			return nil, fmt.Errorf("note get: vault %s: %w", p, err)
		}
		resolved, err := graph.NewResolver(vdb.DB).Resolve(input)
		vdb.Close()
		if err != nil {
			return nil, fmt.Errorf("note get: resolving in %s: %w", p, err)
		}
		if resolved.Resolved {
			holders = append(holders, p)
		}
	}
	return holders, nil
}

// noteGetVaultError reports a miss in every vault, or a note held by several.
func noteGetVaultError(cmd *cobra.Command, input string, paths, holders []string) error {
	w := cmd.OutOrStdout()
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteJson)
	if len(holders) == 0 {
		names := make([]string, len(paths))
		for i, p := range paths {
			names[i] = vaultDisplayName(p)
		}
		msg := fmt.Sprintf("no note matches %q in %s", input, strings.Join(names, ", "))
		if jsonOut {
			return envelope.WriteError(w, envelope.Error("note get", "not_found", msg, ""))
		}
		if _, err := fmt.Fprintf(w, "No note found for %q in %s\n", input, strings.Join(names, ", ")); err != nil {
			return err
		}
		return envelope.ErrAlreadyWritten
	}
	msg := fmt.Sprintf("%q is in %d vaults: %s; read it with --vault <one of them>",
		input, len(holders), strings.Join(holders, ", "))
	if jsonOut {
		env := envelope.Error("note get", "ambiguous_vault", msg, "")
		env.Errors[0].Candidates = holders
		return envelope.WriteError(w, env)
	}
	return fmt.Errorf("%s", msg)
}
