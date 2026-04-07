package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var lintFixLinksCmd = MustNewCommand(commands.LintFixLinksMetadata, runLintFixLinks)

func init() {
	lintCmd.AddCommand(lintFixLinksCmd)
	setupCommandConfig(lintFixLinksCmd)
}

func runLintFixLinks(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLintfixlinksVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLintfixlinksJson)
	fix := getConfigValueWithFlags[bool](cmd, "fix", config.KeyAppLintfixlinksFix)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "lint fix-links")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	result, err := mutation.FixWikilinks(vdb.DB, vaultPath, fix)
	if err != nil {
		return fmt.Errorf("fix-links: %w", err)
	}

	if jsonOut {
		env := envelope.OK("lint fix-links", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	w := cmd.OutOrStdout()
	mode := "dry-run"
	if fix {
		mode = "fix"
	}
	if _, err = fmt.Fprintf(w, "Mode: %s\nFiles scanned: %d\nFiles changed: %d\nLinks fixed: %d\n",
		mode, result.FilesScanned, result.FilesChanged, result.LinksFixed); err != nil {
		return err
	}
	for _, d := range result.Details {
		if _, err = fmt.Fprintf(w, "  %s: %s → %s\n", d.Path, d.OldLink, d.NewLink); err != nil {
			return err
		}
	}
	return nil
}
