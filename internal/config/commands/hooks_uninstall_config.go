package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// HooksUninstallMetadata defines the metadata for the
// `vaultmind hooks uninstall` subcommand — the inverse of
// `hooks install --merge`. It strips VaultMind's hook entries from a
// project's hook config, removing ONLY entries that reference our
// canonical scripts. A project's own hooks and any unrelated settings
// are left untouched.
//
// Per the extend-don't-overwrite principle, vaultmind never silently mutates
// user files beyond its own footprint: uninstall is surgical, and
// --remove-scripts (off by default) is the explicit opt-in to also
// delete the installed script files.
var HooksUninstallMetadata = config.CommandMetadata{
	Use:   "uninstall [project-dir]",
	Short: "Remove VaultMind's hook entries from a project (Claude Code or Codex)",
	Long: `Strip VaultMind's hook entries from a project's Claude Code hook config, or
with --agent codex from .codex/hooks.json.
The inverse of "hooks install --merge": it removes only the entries that
reference VaultMind's canonical scripts, leaving a project's own hooks and
all unrelated settings (permissions, etc.) intact.

PROJECT-DIR

  Directory of the project to clean. Defaults to the current working
  directory if omitted.

WHAT GETS REMOVED

  Any hook entry whose command invokes one of VaultMind's scripts
  (load-persona.sh, vault-recall.sh, vault-track-read.sh,
  capture-episode.sh, and the auto-RAG helpers). An event array left
  empty is dropped, and the "hooks" object is dropped if it becomes
  empty. Entries that merely have a similar name are NOT matched —
  removal is anchored to the exact script path, so a project's own
  hooks are never deleted.

  --local: clean .claude/settings.local.json instead of the default
  .claude/settings.json. Run once per file if you wired into both.

  --remove-scripts: also delete the installed scripts under
  .claude/scripts/ (Codex: .vaultmind/scripts/). Off by default — uninstall
  touches only the hook config unless you ask for the scripts too.

  --agent codex: clean .codex/hooks.json instead, and with --remove-scripts
  .vaultmind/scripts/. The .vaultmind folder itself is never removed (it can
  also hold a vault's data), and Codex's own approval records are not touched.

EXAMPLES

  vaultmind hooks uninstall                            # strip our entries from settings.json
  vaultmind hooks uninstall --local                    # strip from settings.local.json
  vaultmind hooks uninstall --remove-scripts           # also delete .claude/scripts/*.sh
  vaultmind hooks uninstall --agent codex              # strip our entries from .codex/hooks.json
  vaultmind hooks uninstall ~/dev/myproject --json     # machine-readable output`,
	ConfigPrefix: "app.hooksuninstall",
	FlagOverrides: map[string]string{
		"app.hooksuninstall.json":          "json",
		"app.hooksuninstall.local":         "local",
		"app.hooksuninstall.removescripts": "remove-scripts",
		"app.hooksuninstall.agent":         "agent",
	},
}

// HooksUninstallOptions returns configuration options for the hooks
// uninstall subcommand. The project-dir argument is positional.
func HooksUninstallOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.hooksuninstall.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
		{
			Key:          "app.hooksuninstall.local",
			DefaultValue: false,
			Description:  "Target .claude/settings.local.json instead of .claude/settings.json.",
			Type:         "bool",
		},
		{
			Key:          "app.hooksuninstall.removescripts",
			DefaultValue: false,
			Description:  "Also delete the installed hook scripts under .claude/scripts/ (Codex: .vaultmind/scripts/; default: leave them).",
			Type:         "bool",
		},
		{
			Key:          "app.hooksuninstall.agent",
			DefaultValue: "claude",
			Description:  "Agent to unwire: claude (default; .claude/settings.json) or codex (.codex/hooks.json).",
			Type:         "string",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(HooksUninstallOptions)
}
