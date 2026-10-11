package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// HooksStatusMetadata defines `vaultmind hooks status` — the read-only
// comparison between the hook scripts this binary ships and the ones a project
// has installed.
//
// The `hooks` help has advertised drift-checking and status since the command
// existed, while only install and uninstall were implemented. More to the
// point: doctor reported DRIFT but nothing reported ABSENCE, so a hook the
// binary ships and the project never installed looked exactly like a hook that
// matched. Two hooks stayed missing from every adopter for months that way.
var HooksStatusMetadata = config.CommandMetadata{
	Use:   "status [project-dir]",
	Short: "Check installed hook scripts and wiring at project or user scope",
	Long: `Compare the hook scripts a project has installed against the canonical
copies embedded in this binary, and report what differs.

PROJECT-DIR

  Directory of the project to check. Defaults to the current working
  directory if omitted.

SCOPE (--scope project|user)

  project (default): check the selected project. Also compare its wiring with
  user-scope wiring for Claude Code, Codex and Cursor. Overlapping scripts on
  the same event are reported with both settings paths and fail status.
  user: check ~/.vaultmind/scripts against canonical scripts and the wiring
  in ~/.claude/settings.json. Also check Codex user hooks ($CODEX_HOME/hooks.json,
  default ~/.codex/hooks.json), including approvals, and ~/.cursor/hooks.json
  when present. The project-dir argument (default: current directory) selects
  the project to compare for overlaps; user scripts/settings still live in HOME.
  Every wired script path must exist; project-variable paths are broken at user
  scope. Broken entries name the exact command and a fix. User profiles are recorded
  in ~/.vaultmind/vaultmind-profile; undeclared older installs are judged as full.
  Refresh differing scripts with hooks install --scope user --force --merge;
  preview first with --dry-run.

STATES

  in sync   the installed copy behaves identically to the canonical one.
            Comments and blank lines are ignored — a hook's behaviour is
            its code, and flagging annotations would make the whole
            report noise.

  drifted   the installed copy differs behaviourally. Either this project
            changed it — in which case the change should go upstream, or
            an update will overwrite it — or the binary moved ahead and
            the project should update.

  missing   this binary ships the hook and the project does not have it.
            Reported separately because an absence that renders as
            nothing looks exactly like health.

EXIT CODE

  0 when all expected scripts are in sync and events are wired, and Codex
  hooks are approved. Non-zero for drifted, missing, unwired, broken or
  overlapping hooks, so this is usable in a check: a status command that
  always exits 0 cannot gate anything.

EXAMPLES

  vaultmind hooks status                     # check the current project
  vaultmind hooks status ~/dev/myproject     # check another project
  vaultmind hooks status --scope user        # inspect hooks across all projects
  vaultmind hooks status --json              # machine-readable
  vaultmind hooks install --force .          # the fix for drift`,
	ConfigPrefix: "app.hooksstatus",
	FlagOverrides: map[string]string{
		"app.hooksstatus.scope": "scope",
		"app.hooksstatus.json":  "json",
	},
}

// HooksStatusOptions returns configuration options for `hooks status`.
func HooksStatusOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.hooksstatus.scope", DefaultValue: "project", Description: "Hook scope: project or user (across projects)", Type: "string"},
		{
			Key:          "app.hooksstatus.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(HooksStatusOptions)
}
