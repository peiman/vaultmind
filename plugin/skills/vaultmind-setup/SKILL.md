---
name: vaultmind-setup
description: Set up VaultMind for this project. Initialize a knowledge vault, import project Markdown, embed it, and verify retrieval.
---

Set up VaultMind for this project. Work from the project root. Inspect existing
vaults first: reuse one if present; never replace existing notes or configuration.
The plugin already supplies knowledge-profile hooks and MCP tools.

The wrapper is available as `vaultmind` on the Bash tool's PATH. The first call
fetches the pinned binary and verifies its checksum. On darwin_arm64/linux_amd64
it includes ORT. The first embed downloads BGE-M3 (about 2.3 GB); report progress
and allow it to complete. Other macOS/Linux platforms use MiniLM.

Before running the commands, run `export CLAUDE_PLUGIN_DATA="${CLAUDE_PLUGIN_DATA}"`
in the same Bash call so model downloads use the persistent plugin data directory.
Claude Code substitutes the path in this skill; its Bash tool does not otherwise
export plugin path variables.

Run these canonical Quickstart steps (adjust the vault path if reusing a vault):

```bash
vaultmind init ./knowledge
vaultmind import docs *.md --vault ./knowledge
vaultmind index --embed --vault ./knowledge
vaultmind ask "what did we decide about retries?" --vault ./knowledge
```

The shell expands *.md to the root Markdown (README, CONTRIBUTING, AGENTS, and the rest) and skips dotfiles, so tool chat logs such as .aider.chat.history.md stay out; drop docs if the project has no docs/, and if the project has no root Markdown, leave out *.md (zsh aborts the whole command with no matches found otherwise).

Choose a verification question grounded in the project's actual documents if it
has no retry decision. Check that `ask` returns relevant note IDs and content.
Report the vault path, imported sources, embedding backend, and verification result.
For a project without Markdown, create a useful first note with the agent's and
user's project knowledge before embedding. Do not install project hooks: the
plugin provides them. Custom vault paths can be supplied through VAULTMIND_VAULT
for hooks; set the MCP server's project vault explicitly if using a custom path.
After setup, reconnect the vaultmind MCP server or run /reload-plugins if its
initial instructions still say no vault; tools can use ./knowledge immediately.
