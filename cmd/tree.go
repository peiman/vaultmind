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

// treeResult is the JSON result: one map per vault. For is the file the map
// was narrowed to (repo:path), when --for was given.
type treeResult struct {
	For    string      `json:"for,omitempty"`
	Vaults []treeVault `json:"vaults"`
}

// treeQuery is what to map: a filtered vault, or the notes covering one file.
type treeQuery struct {
	filter  navigate.Filter
	forFile *navigate.CodeFile
}

func runTree(cmd *cobra.Command, _ []string) error {
	paths, err := treeVaultPaths(cmd)
	if err != nil {
		return err
	}
	q := treeQueryFromFlags(cmd)
	result := treeResult{For: q.label()}
	for _, p := range paths {
		m, err := loadTreeVault(cmd, p, q)
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

func treeQueryFromFlags(cmd *cobra.Command) treeQuery {
	q := treeQuery{filter: navigate.Filter{
		PathPrefix: getConfigValueWithFlags[string](cmd, "path", config.KeyAppTreePath),
		Type:       getConfigValueWithFlags[string](cmd, "type", config.KeyAppTreeType),
	}}
	if file := getConfigValueWithFlags[string](cmd, "for", config.KeyAppTreeFor); file != "" {
		f := navigate.ResolveCodeFile(file)
		q.forFile = &f
	}
	return q
}

// label names the --for file the way paths: sees it: repo:path, or the path
// alone outside a repository.
func (q treeQuery) label() string {
	if q.forFile == nil {
		return ""
	}
	if q.forFile.Repo == "" {
		return q.forFile.Rel
	}
	return q.forFile.Repo + ":" + q.forFile.Rel
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

func loadTreeVault(cmd *cobra.Command, vaultPath string, q treeQuery) (treeVault, error) {
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, treeEnvelope)
	if err != nil {
		return treeVault{}, err
	}
	defer vdb.Close()
	var notes []navigate.Note
	if q.forFile != nil {
		notes, err = navigate.Covering(vdb.DB, *q.forFile)
	} else {
		notes, err = navigate.Load(vdb.DB, q.filter)
	}
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
		if _, err := fmt.Fprintln(w, treeHeader(v, result.For)); err != nil {
			return err
		}
		if v.Total == 0 {
			if _, err := fmt.Fprintln(w, treeEmpty(v, result.For)); err != nil {
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

func treeHeader(v treeVault, forLabel string) string {
	if forLabel != "" {
		return fmt.Sprintf("%s — %d notes about %s", v.Vault, v.Total, forLabel)
	}
	return fmt.Sprintf("%s — %d notes", v.Vault, v.Total)
}

func treeEmpty(v treeVault, forLabel string) string {
	if forLabel != "" {
		return "  No notes cover " + forLabel + ". A note names the code it is about with paths: in its frontmatter."
	}
	return "  No notes match. If the vault has notes, it may not be indexed: vaultmind index --vault " + v.Vault
}
