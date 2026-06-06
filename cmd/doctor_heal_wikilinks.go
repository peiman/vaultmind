package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// doctorHealWikilinksCmd is the surgical wikilink repair — the moved logic of
// the old `lint fix-links`. It applies by default; --dry-run previews. The
// `fix` alias makes `doctor fix wikilinks` resolve here. It shares the
// runWikilinkFix engine (internal/mutation.FixWikilinks) with `doctor heal`.
var doctorHealWikilinksCmd = func() *cobra.Command {
	c := MustNewCommand(commands.DoctorHealWikilinksMetadata, runDoctorHealWikilinks)
	c.Aliases = []string{"fix"}
	return c
}()

func init() {
	doctorHealCmd.AddCommand(doctorHealWikilinksCmd)
	setupCommandConfig(doctorHealWikilinksCmd)
}

// runDoctorHealWikilinks resolves the wikilinks flags and applies (or, under
// --dry-run, previews) the Obsidian-incompatible wikilink rewrite via the
// shared runWikilinkFix engine.
func runDoctorHealWikilinks(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDoctorhealwikilinksVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorhealwikilinksJson)
	dryRun := getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppDoctorhealwikilinksDryRun)
	return runWikilinkFix(cmd, vaultPath, jsonOut, !dryRun, healModeLabel(!dryRun), "doctor heal wikilinks")
}
