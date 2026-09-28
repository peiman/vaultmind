package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// InitMetadata defines the metadata for the init command — scaffolds a
// fresh vault at a user-provided path.
//
// The knowledge vault is the product: a curated knowledge base for a project
// or across repos, found by search and by the code hooks. That is the default
// scaffold. The agent-identity (persona) vault is an add-on, one flag away.
var InitMetadata = config.CommandMetadata{
	Use:   "init [path]",
	Short: "Scaffold a fresh vault — a project knowledge base by default, or an agent's identity vault",
	Long: `Create a new VaultMind vault at <path>: a type registry, a vault-level
README, and starter notes that demonstrate the schema. After init, you
index, embed, and you're ready.

By default it is a project knowledge base. --profile persona (or full)
scaffolds an agent's identity vault instead.

WHAT YOU GET (default: knowledge)

  <path>/
    .vaultmind/config.yaml         Type registry — decision, concept, source,
                                   reference, and the identity types
    README.md                      How to use the vault
    decisions/decision-example.md  Template — a decision and why
    concepts/concept-example.md    Template — a concept, with an optional
                                   paths: list tying it to code

WITH --profile persona (or full)

  <path>/
    .vaultmind/config.yaml         The same type registry
    README.md                      The persona model + workflow
    identity/who-am-i.md           Foundational identity note (placeholder)
    references/current-context.md  Live-edge priority note (placeholder)
    principles/principle-example.md  Template — replace or delete
    arcs/arc-example.md            Template — replace or delete

The starter notes have today's date in their frontmatter so the index
reads them cleanly without hand-editing. Replace or delete them as real
content arrives.

EXAMPLES

  vaultmind init ./knowledge
      A project knowledge base at a relative path.

  vaultmind init ./knowledge --wire-hooks
      Scaffold AND wire Claude Code in one step (see ONE-COMMAND SETUP).

  vaultmind init "$HOME/.vaultmind/persona" --profile persona
      An agent's identity vault, outside the project tree — common when
      the agent's memory should persist across repos.

  vaultmind init --print-instructions
      Print the concise agent-onboarding quick-start and exit. No vault
      created. The quick-start is the skimmable 20% — install, init,
      hooks, env-var routing, index/embed, first ask. Add --full to
      print the whole guide (preflight, project read, greenfield +
      migration paths, Claude Code wiring with diff-preview) instead.
      Use this when an AI agent is helping a new user wire vaultmind
      into their environment — paste the output to the agent.

  vaultmind init --print-instructions --full
      Print the full agent-onboarding guide instead of the quick-start.

ONE-COMMAND SETUP (--wire-hooks)

  --wire-hooks does the Claude Code wiring for you right after the
  scaffold: it installs the hook scripts into the current project's
  .claude/scripts/ and merges the hook entries into
  .claude/settings.json, baked to the new vault via VAULTMIND_VAULT.
  The merge is additive and NEVER clobbers — a project's own hooks are
  preserved, re-runs are no-ops, malformed settings error out before any
  write. (This is the same engine as "vaultmind hooks install --merge".)

  Which hooks: the --profile you name; else the profile the project
  already declared; else the profile of the vault init just made. So a
  fresh project gets knowledge hooks for a knowledge vault, and a project
  that declared its hooks keeps them.

    --local     wire .claude/settings.local.json (gitignored, personal)
                instead of the committed settings.json
    --dry-run   preview the settings merge without writing it

  The vault is created at <path>; the hooks are wired into the CURRENT
  directory's .claude/ (your project root). After wiring, index + embed
  so recall has something to retrieve, then restart Claude Code. To undo:
  vaultmind hooks uninstall.

NEXT STEPS

  cd <path>
  vaultmind index --vault .            # build the SQLite index
  vaultmind index --embed --vault .    # compute embeddings (one-time)
  vaultmind import ../docs --vault .   # bring in a project's existing docs
  vaultmind ask "what did we decide about X" --vault .

  For agent-led setup (interview, project read, migration, hooks),
  run: vaultmind init --print-instructions

THE PERSONA MODEL

With --profile persona, VaultMind treats arcs — transformation notes — as
the atomic unit of an agent's identity. Identity is carried by the
journey, not by the rules. The persona scaffold gives you placeholder
identity + current-context notes and templates for principles and arcs;
let your real collaboration produce the rest.`,
	ConfigPrefix: "app.init",
	FlagOverrides: map[string]string{
		"app.init.print_instructions": "print-instructions",
		"app.init.full":               "full",
		"app.init.wire_hooks":         "wire-hooks",
		"app.init.profile":            "profile",
		"app.init.local":              "local",
		"app.init.dry_run":            "dry-run",
		"app.init.project_dir":        "project-dir",
	},
}

// InitOptions returns configuration options for the init command.
// The path argument is positional; --print-instructions is the one
// flag, which prints the embedded onboarding doc and exits without
// creating a vault. The flag is the answer to AX-design Q2 ("where
// does the doc live?") — embedded in the binary, accessible
// wherever vaultmind is installed.
func InitOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.init.print_instructions",
			DefaultValue: false,
			Description:  "Print the concise agent-onboarding quick-start and exit (no vault created); add --full for the whole guide",
			Type:         "bool",
		},
		{
			Key:          "app.init.full",
			DefaultValue: false,
			Description:  "With --print-instructions, print the full agent-onboarding guide instead of the concise quick-start",
			Type:         "bool",
		},
		{
			Key:          "app.init.wire_hooks",
			DefaultValue: false,
			Description:  "After scaffolding, install the Claude Code hook scripts into the current project and merge the wiring into .claude/settings.json (baked to the new vault). Never clobbers existing hooks.",
			Type:         "bool",
		},
		{
			Key:          "app.init.profile",
			DefaultValue: "",
			Description:  "What the vault is for: knowledge (the default) scaffolds a project knowledge base (decisions/, concepts/, notes tied to code with paths:); persona or full scaffold an agent's identity vault. With --wire-hooks it is also the hook profile installed; unset, the project's declared profile is kept, else the scaffold's.",
			Type:         "string",
		},
		{
			Key:          "app.init.local",
			DefaultValue: false,
			Description:  "With --wire-hooks, merge into .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).",
			Type:         "bool",
		},
		{
			Key:          "app.init.dry_run",
			DefaultValue: false,
			Description:  "With --wire-hooks, print the would-be settings merge without writing it (preview).",
			Type:         "bool",
		},
		{
			Key:          "app.init.project_dir",
			DefaultValue: "",
			Description:  "With --wire-hooks, the project to wire hooks into (where .claude/ lives). Defaults to the current directory; set it when the vault and the project root differ.",
			Type:         "string",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(InitOptions)
}
