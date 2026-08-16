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
	Short: "Compare a project's installed hook scripts against the canonical ones",
	Long: `Compare the hook scripts a project has installed against the canonical
copies embedded in this binary, and report what differs.

PROJECT-DIR

  Directory of the project to check. Defaults to the current working
  directory if omitted.

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

  0 when every canonical script is in sync. Non-zero when anything is
  drifted or missing, so this is usable in a check: a status command that
  always exits 0 cannot gate anything.

EXAMPLES

  vaultmind hooks status                     # check the current project
  vaultmind hooks status ~/dev/myproject     # check another project
  vaultmind hooks status --json              # machine-readable
  vaultmind hooks install --force .          # the fix for drift`,
	ConfigPrefix: "app.hooksstatus",
	FlagOverrides: map[string]string{
		"app.hooksstatus.json": "json",
	},
}

// HooksStatusOptions returns configuration options for `hooks status`.
func HooksStatusOptions() []config.ConfigOption {
	return []config.ConfigOption{
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
