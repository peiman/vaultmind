package commands

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/memory"
)

// NotePinnedMetadata defines metadata for note pinned.
var NotePinnedMetadata = config.CommandMetadata{
	Use:   "pinned",
	Short: "Load notes marked pinned: true",
	Long: `Load every note whose frontmatter says pinned: true.

Notes are ordered by id within a vault, and vaults are read in the order given
to --vaults. One --budget covers every vault (default 1500). A note that does
not fit is named by id, title, and vault. It is never dropped without a name.

--excerpt N caps each note the same way arc recite does. 0 means the whole
body. The session-start hook passes 400, so a long note arrives as its opening.

With no pinned notes, the command prints nothing and exits 0, so a session-start
hook can call it every time.

The mark lives in the note, so it survives hooks install --force. Run
hooks install --force once to pick up the hook that loads them.

FLAGS

  --vault <dir>    Path to the vault root.
  --vaults a,b,c   Several vaults, comma-separated. Overrides --vault.
                   One budget across all of them.
  --budget N       Token ceiling for the whole run. 0 means unbounded.
  --excerpt N      Cap each note at N tokens. 0 means the whole body.
  --json           Output a v1 envelope.`,
	ConfigPrefix: "app.notepinned",
	FlagOverrides: map[string]string{
		"app.notepinned.vault":   "vault",
		"app.notepinned.vaults":  "vaults",
		"app.notepinned.json":    "json",
		"app.notepinned.budget":  "budget",
		"app.notepinned.excerpt": "excerpt",
	},
}

// NotePinnedOptions returns config options for note pinned.
func NotePinnedOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.notepinned.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.notepinned.vaults", DefaultValue: "", Description: "Load pinned notes from several vaults, comma-separated (overrides --vault). One budget across all of them", Type: "string"},
		{Key: "app.notepinned.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.notepinned.budget", DefaultValue: memory.SessionPinnedBudget, Description: "Total token ceiling for every vault in the run. 0 means unbounded. Notes that do not fit are named by id, title, and vault", Type: "int"},
		{Key: "app.notepinned.excerpt", DefaultValue: 0, Description: "Cap each note at N tokens, using the same excerpt rule as arc recite. 0 means the whole body", Type: "int"},
	}
}

func init() {
	config.RegisterOptionsProvider(NotePinnedOptions)
}
