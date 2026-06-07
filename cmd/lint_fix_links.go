package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// lint fix-links is DEPRECATED: the wikilink repair moved to the doctor health
// hub as `doctor heal wikilinks`. This hidden alias prints a one-line stderr
// notice and delegates to the SAME shared fixer engine (runWikilinkFix →
// internal/mutation.FixWikilinks). It preserves the OLD contract — dry-run by
// default, --fix to apply, and the "Mode: fix"/"Mode: dry-run" labels — so
// existing scripts keep working. Kept for ~2 releases.
var lintFixLinksCmd = newDeprecatedAlias(commands.LintFixLinksMetadata,
	"vaultmind: 'lint fix-links' is deprecated; use 'doctor heal wikilinks' instead",
	runLintFixLinks)

func init() {
	lintCmd.AddCommand(lintFixLinksCmd)
	setupCommandConfig(lintFixLinksCmd)
}

// runLintFixLinks resolves the alias's own app.lintfixlinks.* flags and
// delegates to the shared wikilink-fix engine. --fix (default false) maps to
// apply; without it the alias previews (dry-run) — the old contract, the
// inverse of the new heal default.
func runLintFixLinks(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLintfixlinksVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLintfixlinksJson)
	apply := getConfigValueWithFlags[bool](cmd, "fix", config.KeyAppLintfixlinksFix)
	return runWikilinkFix(cmd, vaultPath, jsonOut, apply, fixLinksModeLabel(apply), "lint fix-links")
}

// fixLinksModeLabel preserves the legacy fix-links human-output labels:
// "fix" when applying, "dry-run" when previewing. (heal uses "apply" instead;
// both share the same engine, only the label differs.)
func fixLinksModeLabel(apply bool) string {
	if apply {
		return "fix"
	}
	return "dry-run"
}
