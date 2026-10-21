# VaultMind for Claude Code

Install from your shell (Claude Code 2.1.292 or newer):

```sh
claude plugin install vaultmind --marketplace peiman/vaultmind
```

In Claude Code, run `/vaultmind:vaultmind-setup`. The agent creates `./knowledge`,
imports `docs/` and root Markdown, embeds the notes, and verifies an answer.
The plugin supplies knowledge-profile hooks: session health and pinned notes,
prompt recall, read tracking, decision-time recall, code maps, and preservation
before compaction. It also exposes VaultMind's MCP tools.

On first use, `bin/vaultmind` reuses a matching-version executable from PATH or
fetches the pinned GitHub release, verifies SHA-256, and atomically caches it.
Apple Silicon macOS and Linux amd64 get ORT; other macOS/Linux platforms get
MiniLM. Keep the bundled `libonnxruntime*` files beside the executable. The first
ORT embed downloads BGE-M3 (about 2.3 GB). Hooks require bash and Python 3; release
downloads require curl or wget and sha256sum or shasum. Native Windows launching
is not supported by this POSIX wrapper.

Binaries live in `${CLAUDE_PLUGIN_DATA}/<version>/`. Updates preserve this data.
Claude Code exposes `bin/` on the Bash tool's PATH; hook commands also explicitly
prepend it. Bash-tool commands do not receive `CLAUDE_PLUGIN_ROOT` or
`CLAUDE_PLUGIN_DATA`, so the wrapper derives its root from its own location and
maps `plugins/cache/<marketplace>/<plugin>/<version>` to
`plugins/data/<plugin>-<marketplace>` (identifier punctuation becomes `-`).
Local development outside that layout uses `~/.cache/vaultmind-plugin` for binaries.

The wrapper preserves `XDG_CACHE_HOME`. Models use VaultMind's standard shared
user cache, so an existing BGE-M3 download is reused by every install:
`~/Library/Caches/vaultmind/models/BAAI_bge-m3` on macOS and
`~/.cache/vaultmind/models/BAAI_bge-m3` on Linux, or
`$XDG_CACHE_HOME/vaultmind/models/BAAI_bge-m3` when configured.

Without a vault, SessionStart names the setup skill and MCP stays connected,
reporting "no vault in this project". Hooks and MCP discover `knowledge`,
`vaultmind-vault`, `vaultmind-identity`, or a vault at the project root, in that
order. Existing project hook installations keep their own vault configuration.
For other vault locations, use `VAULTMIND_VAULT` for hooks and configure the MCP
server with explicit `--vault <path>` arguments. Reconnect MCP after changing
its vault location.

## Existing hooks

The plugin marks hook invocations with `VAULTMIND_HOOK_SOURCE=plugin`. Each
script yields quietly if `.claude/settings.json` or `.claude/settings.local.json`
already wires that script under `.claude/scripts/`. It also checks user-scope
`$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude/settings.json`) for
`/.vaultmind/scripts/<name>`. Installed hooks still run once;
the plugin supplies any hooks the project does not have. You can keep the project
installation, or run `vaultmind hooks uninstall .` to use only the plugin.
The setup skill does not install another set of project hooks.

## Uninstall

```sh
claude plugin uninstall vaultmind@vaultmind
```

Uninstalling from the last scope deletes `${CLAUDE_PLUGIN_DATA}`, including the
downloaded binary. Add `--keep-data` to retain it. The shared model stays in
the user cache. To remove BGE-M3 manually, delete
`~/Library/Caches/vaultmind/models/BAAI_bge-m3` on macOS or
`~/.cache/vaultmind/models/BAAI_bge-m3` on Linux (use
`$XDG_CACHE_HOME/vaultmind/models/BAAI_bge-m3` if set). Other installs using that
cache will need to download it again. Project vault notes
remain in the project. If you also installed project hooks with `hooks install`,
those are independent; remove them with `vaultmind hooks uninstall .` if desired.

## Release maintenance

See [CONTRIBUTING](https://github.com/peiman/vaultmind/blob/main/CONTRIBUTING.md).
The manifest version, Unreleased release intent, embedded script copies, hook
wiring and setup commands are guarded by Go tests. Test downloads can use
`VAULTMIND_PLUGIN_RELEASE_URL` as a release base URL (the wrapper appends
`/v<version>/<asset>`). Production defaults to GitHub releases.
