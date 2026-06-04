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
// Per the extend-don't-overwrite principle, vaultmind never silently
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

  All 8 embedded scripts are written to .claude/scripts/. Four are the
  core hooks --merge wires into your settings; the rest are helpers the
  core scripts source or that you opt into separately.

  Core (wired by --merge):
    load-persona.sh       SessionStart — loads identity context
    vault-recall.sh       UserPromptSubmit — per-turn pointers
    vault-track-read.sh   PreToolUse(Read) — access tracking
    capture-episode.sh    SessionEnd — episode transcript capture

  Helpers (written, not wired by default):
    vault-block-read.sh   parked variant — block-and-redirect on Read
    auto-rag-guard.sh     PreToolUse drift gate (opt-in — see auto-RAG)
    auto-rag-evaluate.sh  scores auto-RAG guard decisions
    shell-strip.sh        shared helper sourced by the auto-RAG scripts

After writing the scripts, this command prints the .claude/settings.json
"hooks" stanza that wires them. Pass --merge to apply it for you instead
of copy-pasting (see WIRING below).

  --vault: point the hooks at a specific vault. The stanza bakes
  VAULTMIND_VAULT=<path> into every command so recall, read-tracking,
  episode capture, and persona loading all target that vault instead of
  the built-in default ($CLAUDE_PROJECT_DIR/vaultmind-identity). Omit it
  to keep the default convention.

WIRING (--merge)

  --merge additively merges the four canonical hook entries straight into
  the project's hook config — no hand-editing. The merge NEVER clobbers:
  a project's own hooks are preserved, our entries are de-duplicated (so
  re-running is a no-op), all other settings and key order are kept, and
  malformed settings error out before any write. Pair with
  "hooks uninstall" to cleanly remove only our entries later.

  --local: merge into .claude/settings.local.json (gitignored, personal)
  instead of .claude/settings.json (committed, team-shared). Choose local
  for a personal persona vault; the default for a capability the whole
  team should get.

  --dry-run: with --merge, print the merged result without writing it —
  use it to preview/diff before applying.

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
  vaultmind hooks install --only auto-rag-guard.sh,shell-strip.sh      # only the auto-RAG slice (companion-project pattern)
  vaultmind hooks install --vault ./my-knowledge                       # print a stanza wired to a specific vault
  vaultmind hooks install --vault ./my-knowledge --merge --dry-run     # preview the merge without writing
  vaultmind hooks install --vault ./my-knowledge --merge               # write scripts AND wire settings.json
  vaultmind hooks install --vault ./my-knowledge --merge --local       # wire personal settings.local.json instead`,
	ConfigPrefix: "app.hooksinstall",
	FlagOverrides: map[string]string{
		"app.hooksinstall.force":  "force",
		"app.hooksinstall.json":   "json",
		"app.hooksinstall.only":   "only",
		"app.hooksinstall.vault":  "vault",
		"app.hooksinstall.merge":  "merge",
		"app.hooksinstall.local":  "local",
		"app.hooksinstall.dryrun": "dry-run",
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
		{
			Key:          "app.hooksinstall.vault",
			DefaultValue: "",
			Description:  "Vault path to bake into the printed settings.json stanza via VAULTMIND_VAULT (default: the built-in vaultmind-identity convention).",
			Type:         "string",
		},
		{
			Key:          "app.hooksinstall.merge",
			DefaultValue: false,
			Description:  "Additively merge the hook stanza into the project's settings file (never clobbers existing hooks) instead of only printing it.",
			Type:         "bool",
		},
		{
			Key:          "app.hooksinstall.local",
			DefaultValue: false,
			Description:  "With --merge, target .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).",
			Type:         "bool",
		},
		{
			Key:          "app.hooksinstall.dryrun",
			DefaultValue: false,
			Description:  "With --merge, print the merged result without writing it (preview/diff).",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(HooksInstallOptions)
}
