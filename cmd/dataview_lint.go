package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	marker "github.com/peiman/vaultmind/internal/marker"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

type dataviewLintResult struct {
	FilesChecked int             `json:"files_checked"`
	Valid        int             `json:"valid"`
	Issues       []dataviewIssue `json:"issues"`
}

type dataviewIssue struct {
	Path       string `json:"path"`
	SectionKey string `json:"section_key,omitempty"`
	Rule       string `json:"rule"`
	Message    string `json:"message"`
	Line       int    `json:"line"`
}

var dataviewLintCmd = MustNewCommand(commands.DataviewLintMetadata, runDataviewLint)

func init() {
	dataviewCmd.AddCommand(dataviewLintCmd)
	setupCommandConfig(dataviewLintCmd)
}

func runDataviewLint(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDataviewlintVault)
	useJSON := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDataviewlintJson)

	result, indexHash, err := executeDataviewLint(vaultPath)
	if err != nil {
		return err
	}
	if useJSON {
		return dataviewLintJSON(cmd, result, vaultPath, indexHash)
	}
	return dataviewLintText(cmd, result)
}

func executeDataviewLint(vaultPath string) (dataviewLintResult, string, error) {
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return dataviewLintResult{}, "", err
	}
	defer vdb.Close()

	files, err := vault.Scan(vaultPath, vdb.Config.Vault.Exclude)
	if err != nil {
		return dataviewLintResult{}, "", fmt.Errorf("scanning vault: %w", err)
	}

	result := dataviewLintResult{Issues: []dataviewIssue{}}
	for _, f := range files {
		result.FilesChecked++
		raw, readErr := os.ReadFile(f.AbsPath) //nolint:gosec // path from vault scanner
		if readErr != nil {
			result.Issues = append(result.Issues, dataviewIssue{Path: f.RelPath, Rule: "read_error", Message: readErr.Error()})
			continue
		}
		issues := marker.ValidateMarkers(raw)
		if len(issues) == 0 {
			result.Valid++
			continue
		}
		for _, issue := range issues {
			result.Issues = append(result.Issues, dataviewIssue{
				Path: f.RelPath, SectionKey: issue.SectionKey,
				Rule: issue.Rule, Message: issue.Message, Line: issue.Line,
			})
		}
	}
	return result, vdb.GetIndexHash(), nil
}

func dataviewLintJSON(cmd *cobra.Command, result dataviewLintResult, vaultPath, indexHash string) error {
	env := envelope.OK("dataview lint", result)
	if len(result.Issues) > 0 {
		env.Status = "warning"
	}
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = indexHash
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}

func dataviewLintText(cmd *cobra.Command, result dataviewLintResult) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Checked %d files: %d valid, %d issues\n",
		result.FilesChecked, result.Valid, len(result.Issues)); err != nil {
		return err
	}
	for _, issue := range result.Issues {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", issue.Rule, issue.Path, issue.Message); err != nil {
			return err
		}
	}
	return nil
}
