package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityPathsMetadata defines `vaultmind identity paths` — the one derivation
// of an agent's mesh identity and state paths, emitted for shells and machines.
//
// The canonical wake-watcher's first statement is
//
//	eval "$(vaultmind identity paths)"
//
// and the script contains zero identity literals of its own. That is the point:
// before this command, three hand-maintained watcher copies each carried the
// slug, the daemon URL, and the state paths inline (one heredoc cannot
// interpolate, so the slug alone was spelled four times in one script), and the
// copies drifted — one watcher died for seven days partly because the checker
// and the writer disagreed about a path nobody could see disagreeing.
//
// A binary call cannot be forgotten the way an env var can, fails loudly when
// it cannot answer, and names WHERE each value came from. It is also a live
// preflight of the binary itself: if this works, `vaultmind` works.
var IdentityPathsMetadata = config.CommandMetadata{
	Use:   "paths",
	Short: "Emit this agent's resolved mesh identity and state paths",
	Long: `Resolve this agent's mesh identity — slug, wire principal, daemon URL,
and every wake-watcher state path — and print it.

The slug comes from the agents.yaml registry (AGENT_CHAT_REGISTRY) matched
against the project path (AGENT_CHAT_PROJECT_PATH, else the working
directory), exactly as chat-mcp itself resolves identity. The state paths come
from the same derivation "vaultmind doctor" checks, so the watcher and its
checker cannot disagree about where the heartbeat lives.

FAILS LOUDLY when no slug is resolvable: a watcher armed with an empty
identity would write degenerate state files a checker then reports on. Exit
nonzero means "do not arm".

OUTPUT

  default   shell-quoted VM_MESH_* assignments, safe to eval:
              eval "$(vaultmind identity paths)"
  --json    the same facts in the standard envelope

EXAMPLES

  vaultmind identity paths
  vaultmind identity paths --json`,
	ConfigPrefix: "app.identitypaths",
	FlagOverrides: map[string]string{
		"app.identitypaths.json": "json",
	},
}

// IdentityPathsOptions returns configuration options for `identity paths`.
func IdentityPathsOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.identitypaths.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityPathsOptions)
}
