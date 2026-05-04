package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// InitMetadata defines the metadata for the init command — scaffolds a
// fresh persona-shaped vault at a user-provided path.
//
// VaultMind is built for a human collaborating with an AI agent: the
// agent reads the vault as long-term memory, both human and agent
// curate the markdown. The default scaffold reflects that — types and
// starter notes match the persona-reconstruction model that makes
// VaultMind distinct from a plain notes app.
var InitMetadata = config.CommandMetadata{
	Use:   "init [path]",
	Short: "Scaffold a fresh vault — persona-shaped, ready for you and your agent",
	Long: `Create a new VaultMind vault at <path> with the standard persona-shaped
type registry, a vault-level README, and starter notes that demonstrate
the schema. After init, you index, embed, and you're ready.

WHAT YOU GET

  <path>/
    .vaultmind/config.yaml       Type registry — identity, principle, arc,
                                 reference, concept, source, decision
    README.md                    Vault model + workflow
    identity/who-am-i.md         Foundational identity note (placeholder)
    references/current-context.md  Live-edge priority note (placeholder)
    principles/principle-example.md  Template — replace or delete
    arcs/arc-example.md          Template — replace or delete

The placeholder notes have today's date in their frontmatter so the
index reads them cleanly without hand-editing. Replace the bodies with
your agent's real content as your collaboration produces it.

EXAMPLES

  vaultmind init ./my-vault
      Standard scaffold at a relative path.

  vaultmind init "$HOME/.vaultmind/persona"
      A vault outside the project tree — common when the agent's
      memory should persist across multiple repos.

  vaultmind init --print-instructions
      Print the embedded agent-onboarding script and exit. No vault
      created. Use this when an AI agent is helping a new user wire
      vaultmind into their environment — paste the output to the
      agent and it walks the user through preflight, project read,
      greenfield + migration paths, and Claude Code wiring with
      diff-preview.

NEXT STEPS

  cd <path>
  vaultmind index --vault .            # build the SQLite index
  vaultmind index --embed --vault .    # compute embeddings (one-time)
  vaultmind ask "who am I" --vault .   # see what the agent would see

  For agent-led setup (interview, project read, migration, hooks),
  run: vaultmind init --print-instructions

THE MODEL

VaultMind treats arcs — transformation notes — as the atomic unit of
persona. Identity is carried by the journey, not by the rules. The
default scaffold gives you placeholder identity + current-context
notes and example templates for principles and arcs; let your real
collaboration produce the rest.`,
	ConfigPrefix: "app.init",
	FlagOverrides: map[string]string{
		"app.init.print_instructions": "print-instructions",
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
			Description:  "Print the embedded agent-onboarding script and exit (no vault created)",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(InitOptions)
}
