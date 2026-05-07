package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// HooksInstallMetadata defines the metadata for the
// `vaultmind hooks install` subcommand. Writes the embedded
// Claude Code hook scripts to <project>/.claude/scripts/, where
// the user's hook config (.claude/settings.json) can invoke them.
//
// Default mode refuses to overwrite existing scripts; --force
// overwrites. Refresh after a binary upgrade is the same command
// with --force; doctor's hook-drift check (separate work) flags
// when stale copies need refreshing.
//
// Per arc-extending-not-overwriting, vaultmind never silently
// rewrites user files. Refuse-by-default + explicit --force is
// the same gate `vaultmind init` uses.
var HooksInstallMetadata = config.CommandMetadata{
	Use:   "install [project-dir]",
	Short: "Install Claude Code hook scripts into a project",
	Long: `Write VaultMind's Claude Code hook scripts to <project-dir>/.claude/scripts/.
The scripts are embedded in the vaultmind binary; this command
copies them out for use by Claude Code's hook system.

PROJECT-DIR

  Directory of the project to install hooks into. Defaults to the
  current working directory if omitted. The script writes to
  <project-dir>/.claude/scripts/. Creates the directory if missing.

WHAT GETS INSTALLED

  load-persona.sh       SessionStart — loads identity context
  vault-recall.sh       UserPromptSubmit — per-turn pointers
  vault-track-read.sh   PreToolUse(Read) — access tracking
  vault-block-read.sh   (parked variant — block-and-redirect)
  capture-episode.sh    SessionEnd — episode transcript capture

After install, wire them into .claude/settings.json (the next step
in the agent-led onboarding doc — run 'vaultmind init --print-instructions').

OVERWRITE PROTECTION

  Default: refuses to write any script that already exists. Lists
  the conflicts and exits non-zero so the agent can surface them.

  --force: overwrites existing scripts unconditionally. Use when
  refreshing after a vaultmind binary upgrade (the canonical
  source updates; existing copies become stale).

  --only: comma-separated subset of canonical scripts to install
  (e.g. --only auto-rag-guard.sh,shell-strip.sh,auto-rag-evaluate.sh).
  Consumers who've customized some hooks but want a clean canonical
  install of others — pass --only to scope the install. Unknown
  names rejected at lint time so typos surface explicitly.

  Doctor's hook-drift check (vaultmind doctor) flags when installed
  copies differ from the embedded canonical, so refresh-need is
  surfaced explicitly rather than left to guessing.

EXAMPLES

  vaultmind hooks install                                              # install all 8 into current dir
  vaultmind hooks install ~/dev/myproject                              # install all 8 into a specific project
  vaultmind hooks install --force                                      # refresh after binary upgrade
  vaultmind hooks install --json                                       # machine-readable output
  vaultmind hooks install --only auto-rag-guard.sh,shell-strip.sh      # only the auto-RAG slice (workhorse pattern)`,
	ConfigPrefix: "app.hooksinstall",
	FlagOverrides: map[string]string{
		"app.hooksinstall.force": "force",
		"app.hooksinstall.json":  "json",
		"app.hooksinstall.only":  "only",
	},
}

// HooksInstallOptions returns configuration options for the hooks
// install subcommand. The project-dir argument is positional;
// --force and --json are the flags.
func HooksInstallOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.hooksinstall.force",
			DefaultValue: false,
			Description:  "Overwrite existing hook scripts (default: refuse)",
			Type:         "bool",
		},
		{
			Key:          "app.hooksinstall.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
		{
			Key:          "app.hooksinstall.only",
			DefaultValue: "",
			Description:  "Comma-separated subset of canonical scripts to install (default: all). Unknown names rejected at lint time.",
			Type:         "string",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(HooksInstallOptions)
}
