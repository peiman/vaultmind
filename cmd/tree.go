package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/spf13/cobra"
)

// treeEnvelope names the command in its JSON envelope.
const treeEnvelope = "tree"

var treeCmd = MustNewCommand(commands.TreeMetadata, runTree)

func init() {
	MustAddToRoot(treeCmd)
}

// treeVault is one vault's map in the JSON result.
type treeVault struct {
	Vault string        `json:"vault"`
	Total int           `json:"total"`
	Root  *navigate.Dir `json:"root"`
}

// treeResult is the JSON result: one map per vault.
type treeResult struct {
	Vaults []treeVault `json:"vaults"`
}

func runTree(cmd *cobra.Command, _ []string) error {
	paths, err := treeVaultPaths(cmd)
	if err != nil {
		return err
	}
	filter := navigate.Filter{
		PathPrefix: getConfigValueWithFlags[string](cmd, "path", config.KeyAppTreePath),
		Type:       getConfigValueWithFlags[string](cmd, "type", config.KeyAppTreeType),
	}
	var result treeResult
	for _, p := range paths {
		m, err := loadTreeVault(cmd, p, filter)
		if err != nil {
			return err
		}
		result.Vaults = append(result.Vaults, m)
	}
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppTreeJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK(treeEnvelope, result))
	}
	return writeTree(cmd.OutOrStdout(), result, navigate.RenderOptions{
		Depth: getConfigValueWithFlags[int](cmd, "depth", config.KeyAppTreeDepth),
		Brief: getConfigValueWithFlags[bool](cmd, "brief", config.KeyAppTreeBrief),
	})
}

// treeVaultPaths is --vaults when given (each must already be a vault), else --vault.
func treeVaultPaths(cmd *cobra.Command) ([]string, error) {
	single := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppTreeVault)
	list := getConfigValueWithFlags[string](cmd, "vaults", config.KeyAppTreeVaults)
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

func loadTreeVault(cmd *cobra.Command, vaultPath string, filter navigate.Filter) (treeVault, error) {
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, treeEnvelope)
	if err != nil {
		return treeVault{}, err
	}
	defer vdb.Close()
	notes, err := navigate.Load(vdb.DB, filter)
	if err != nil {
		return treeVault{}, err
	}
	root := navigate.Build(notes)
	return treeVault{Vault: vaultPath, Total: root.Total, Root: root}, nil
}

func writeTree(w io.Writer, result treeResult, o navigate.RenderOptions) error {
	for i, v := range result.Vaults {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s — %d notes\n", v.Vault, v.Total); err != nil {
			return err
		}
		if v.Total == 0 {
			if _, err := fmt.Fprintln(w, "  No notes match. If the vault has notes, it may not be indexed: vaultmind index --vault "+v.Vault); err != nil {
				return err
			}
			continue
		}
		if err := navigate.Render(w, v.Root, o); err != nil {
			return err
		}
	}
	return nil
}
