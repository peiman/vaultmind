// cmd/zz_catalog.go
// ckeletin:allow-custom-command
//
// This file is the SINGLE SOURCE OF TRUTH for the command catalog: the cobra
// group every user-facing command belongs to, its when-to-use trigger phrase,
// and the composition of that trigger into each command's --help (Long) text.
//
// Why a "zz_" filename: command registration happens in per-file init()
// functions scattered across cmd/. Go runs a package's init() functions in
// filename order, so a "zz_"-prefixed file is guaranteed to run LAST — after
// every command has been added to the tree. That lets decorateCommandCatalog
// walk a fully-assembled tree. The four groups themselves are registered in
// root.go's init() (group registration has no subcommand dependency).
//
// The validator (ADR-001) flags this file because it lives in cmd/ and
// references cobra.Command without being a thin command; the whitelist comment
// above opts out. It is catalog wiring, not a command.
package cmd

import (
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/internal/commandcatalog"
	"github.com/spf13/cobra"
)

// Catalog group IDs + titles. These are registered on RootCmd in root.go's
// init() via RootCmd.AddGroup and referenced here when stamping each command's
// GroupID — one definition, two consumers (SSOT).
const (
	groupRetrieval   = "retrieval"
	groupMaintenance = "maintenance"
	groupLifecycle   = "lifecycle"
	groupSetup       = "setup"

	groupRetrievalTitle   = "Retrieval & memory:"
	groupMaintenanceTitle = "Vault maintenance:"
	groupLifecycleTitle   = "Identity & sessions:"
	groupSetupTitle       = "Setup & introspection:"
)

// annotationWhen is the cobra Annotations key under which each command's
// when-to-use trigger phrase is stored. whenToUsePrefix is the header line the
// trigger is composed under in --help (Long) output.
const (
	annotationWhen  = "when"
	whenToUsePrefix = "When to use:"
)

// catalogEntry is one command's catalog metadata: the group it belongs to and
// its when-to-use trigger phrase. The map below keys these by CommandPath()
// (e.g. "vaultmind note get") so the assignment is unambiguous across the tree.
type catalogEntry struct {
	group string
	when  string
	// short, when non-empty, tightens the command's Short (the WHAT line).
	// Empty means "keep the existing Short" — used where the current Short is
	// already a crisp imperative one-liner.
	short string
}

// commandCatalog maps every user-facing command path to its catalog entry.
// Hidden deprecated aliases (links*, lint*, vault*, memory recall/context-pack)
// are intentionally absent — they stay ungrouped and hidden. The enforcement
// test (catalog_test.go) guarantees every non-hidden command has an entry here.
var commandCatalog = map[string]catalogEntry{
	// ── Retrieval & memory ───────────────────────────────────────────────
	"vaultmind ask": {
		group: groupRetrieval,
		when:  "you want to answer \"what do I know about X?\" — search plus packed context in one step.",
	},
	"vaultmind search": {
		group: groupRetrieval,
		when:  "you want a ranked list of hits to browse and pick from, without packed context.",
		short: "Search vault notes by keyword, semantic similarity, or both",
	},
	"vaultmind resolve": {
		group: groupRetrieval,
		when:  "you have a fragment, alias, title, or path and need the canonical note ID.",
		short: "Resolve a fragment, alias, title, or path to canonical note IDs",
	},
	"vaultmind self": {
		group: groupRetrieval,
		when:  "you want to see your own memory state — recent, hot, and stale notes.",
		short: "Show your memory state — recent, hot, and stale notes",
	},
	"vaultmind note": {
		group: groupRetrieval,
		when:  "you want to read, create, or batch-fetch a specific note by ID.",
		short: "Read, create, and batch-fetch notes by ID",
	},
	"vaultmind note get": {
		group: groupRetrieval,
		when:  "you know a note's ID or path and want its full content (with access tracking).",
		short: "Get one note's full content and metadata by ID",
	},
	"vaultmind note create": {
		group: groupRetrieval,
		when:  "you want to create a new note from its type template with field overrides.",
		short: "Create a note from a template with field overrides",
	},
	"vaultmind note mget": {
		group: groupRetrieval,
		when:  "you have several note IDs and want them all in one batched fetch.",
		short: "Fetch multiple notes by ID in one call",
	},
	"vaultmind memory": {
		group: groupRetrieval,
		when:  "you need the low-level graph primitives behind ask: links, neighbors, related, pack, summarize.",
		short: "Traverse the note graph and assemble context for agents",
	},
	"vaultmind memory links": {
		group: groupRetrieval,
		when:  "you want a note's directed wikilink edges — outbound, inbound, or both.",
		short: "List a note's directed wikilink edges (outbound, inbound, or both)",
	},
	"vaultmind memory neighbors": {
		group: groupRetrieval,
		when:  "you want the enriched graph around a note via depth-limited BFS, with full frontmatter.",
		short: "Traverse the graph from a note (BFS) and return enriched neighbors",
	},
	"vaultmind memory related": {
		group: groupRetrieval,
		when:  "you want a simple ranked list of directly connected notes, filtered by edge type.",
		short: "List notes related to a target, filtered by edge type",
	},
	"vaultmind memory pack": {
		group: groupRetrieval,
		when:  "you want a token-budgeted context payload ready to ship to an agent.",
		short: "Pack a note plus ranked context within a token budget",
	},
	"vaultmind memory summarize": {
		group: groupRetrieval,
		when:  "you have a known list of note IDs and want their material assembled for synthesis.",
		short: "Assemble material from specific note IDs for agent synthesis",
	},

	// ── Vault maintenance ────────────────────────────────────────────────
	"vaultmind index": {
		group: groupMaintenance,
		when:  "vault notes changed and you need to refresh the SQLite index (and optionally embeddings).",
		short: "Scan and index vault notes into SQLite, optionally embedding",
	},
	"vaultmind apply": {
		group: groupMaintenance,
		when:  "you have an AI-generated JSON plan and want to execute its note mutations.",
		short: "Execute an AI-generated plan to mutate vault notes",
	},
	"vaultmind schema": {
		group: groupMaintenance,
		when:  "you need to discover the vault's note types, required fields, and valid statuses.",
		short: "Query the vault's type schema",
	},
	"vaultmind schema list-types": {
		group: groupMaintenance,
		when:  "you want every registered type with its required fields and valid statuses before creating notes.",
		short: "List every note type with its required fields and valid statuses",
	},
	"vaultmind frontmatter": {
		group: groupMaintenance,
		when:  "you need to audit, validate, or programmatically edit YAML frontmatter across notes.",
		short: "Inspect and mutate YAML frontmatter across vault notes",
	},
	"vaultmind frontmatter validate": {
		group: groupMaintenance,
		when:  "you want to catch missing fields, bad statuses, unknown types, or broken refs before indexing.",
		short: "Check vault notes for frontmatter rule violations",
	},
	"vaultmind frontmatter fix": {
		group: groupMaintenance,
		when:  "you are migrating notes and need to backfill the missing \"created\" field.",
		short: "Backfill missing \"created\" frontmatter on domain notes",
	},
	"vaultmind frontmatter set": {
		group: groupMaintenance,
		when:  "you want to set a single frontmatter field on one note, schema-validated.",
		short: "Set one frontmatter field on a note",
	},
	"vaultmind frontmatter unset": {
		group: groupMaintenance,
		when:  "you want to remove one frontmatter field from a note.",
		short: "Remove one frontmatter field from a note",
	},
	"vaultmind frontmatter merge": {
		group: groupMaintenance,
		when:  "you want to merge many key/value pairs from a YAML file into one note at once.",
		short: "Merge multiple frontmatter fields from a YAML file into a note",
	},
	"vaultmind frontmatter normalize": {
		group: groupMaintenance,
		when:  "you want to clean up one note's frontmatter formatting — key order, aliases, dates, snake_case.",
		short: "Normalize one note's frontmatter (keys, aliases, dates, snake_case)",
	},
	"vaultmind dataview": {
		group: groupMaintenance,
		when:  "you manage template-generated regions in notes and need to render or lint their markers.",
		short: "Manage template-generated regions in vault notes",
	},
	"vaultmind dataview render": {
		group: groupMaintenance,
		when:  "you edited a template and want to refresh a note's generated region.",
		short: "Render a note's generated region from its template",
	},
	"vaultmind dataview lint": {
		group: groupMaintenance,
		when:  "you want to catch malformed or duplicated VAULTMIND:GENERATED markers before rendering.",
		short: "Scan the vault for broken or duplicate generated-region markers",
	},
	"vaultmind doctor": {
		group: groupMaintenance,
		when:  "you want a read-only health overview of a vault (or every vault, with --all).",
		short: "Vault health hub: diagnose a vault and report issues",
	},
	"vaultmind doctor heal": {
		group: groupMaintenance,
		when:  "doctor found auto-fixable issues and you want to apply every repair at once.",
		short: "Apply all auto-fixable repairs doctor found",
	},
	"vaultmind doctor heal wikilinks": {
		group: groupMaintenance,
		when:  "doctor flagged Obsidian-incompatible wikilinks and you want them rewritten to [[filename|Title]].",
		short: "Rewrite Obsidian-incompatible wikilinks to [[filename|Title]]",
	},

	// ── Identity & sessions ──────────────────────────────────────────────
	"vaultmind init": {
		group: groupLifecycle,
		when:  "you are starting fresh and need to scaffold a new persona-shaped vault.",
		short: "Scaffold a fresh persona-shaped vault, ready for you and your agent",
	},
	"vaultmind episode": {
		group: groupLifecycle,
		when:  "you want to capture Claude Code sessions as episodic-memory artifacts.",
		short: "Capture Claude Code sessions as episodic-memory artifacts",
	},
	"vaultmind episode capture": {
		group: groupLifecycle,
		when:  "you have a session transcript (or a directory of them) to convert into episode notes.",
		short: "Convert session transcripts into episode notes",
	},
	"vaultmind arc": {
		group: groupLifecycle,
		when:  "you want to surface arc-distillation candidate moments from episodes (propose-only).",
		short: "Surface arc-distillation candidates from episodes (propose-only)",
	},
	"vaultmind arc candidates": {
		group: groupLifecycle,
		when:  "you finished a session and want candidate transformation moments to review for arcs.",
		short: "Surface candidate transformation moments for arc distillation",
	},
	"vaultmind identity": {
		group: groupLifecycle,
		when:  "you need Contract-B agent identity: mint a keypair or sign an entry via the keyless signer.",
		short: "Contract-B agent identity: keypair custody and signing",
	},
	"vaultmind identity init": {
		group: groupLifecycle,
		when:  "you are setting up an agent and need to mint its ed25519 keypair and seal the private key to the signer.",
		short: "Mint an agent keypair and seal the private key to the signer",
	},
	"vaultmind identity sign": {
		group: groupLifecycle,
		when:  "you have a Contract-B entry to sign and want it validated, canonicalized, and signed by the keyless signer.",
		short: "Validate, canonicalize, and sign an entry via the keyless signer",
	},
	"vaultmind identity sign-envelope": {
		group: groupLifecycle,
		when:  "you have a chat MESSAGE envelope to sign so a receiving daemon can verify the signature and the signer's registry binding.",
		short: "Sign a chat message envelope via the keyless signer (Contract-B slice 5)",
	},
	"vaultmind hooks": {
		group: groupLifecycle,
		when:  "you need to install, remove, or check VaultMind's Claude Code hook scripts.",
		short: "Manage VaultMind's Claude Code hook scripts",
	},
	"vaultmind hooks install": {
		group: groupLifecycle,
		when:  "you want to wire VaultMind into a project by writing its hook scripts.",
		short: "Install Claude Code hook scripts into a project",
	},
	"vaultmind hooks uninstall": {
		group: groupLifecycle,
		when:  "you want to remove VaultMind's Claude Code hook entries from a project.",
		short: "Remove VaultMind's Claude Code hook entries from a project",
	},

	// ── Setup & introspection ────────────────────────────────────────────
	"vaultmind config": {
		group: groupSetup,
		when:  "you want to manage or validate the application's configuration file.",
		short: "Manage and validate application configuration",
	},
	"vaultmind config validate": {
		group: groupSetup,
		when:  "you want to check a config file for correctness, security, and unknown keys.",
		short: "Validate a configuration file",
	},
	"vaultmind docs": {
		group: groupSetup,
		when:  "you want to generate documentation about the app and its configuration.",
		short: "Generate documentation",
	},
	"vaultmind docs config": {
		group: groupSetup,
		when:  "you want a generated reference of every configuration option.",
		short: "Generate the configuration-options reference",
	},
	"vaultmind docs commands": {
		group: groupSetup,
		when:  "you want a generated grouped reference of every command, each with its when-to-use.",
		short: "Generate the grouped command reference (COMMANDS.md)",
	},
	"vaultmind completion": {
		group: groupSetup,
		when:  "you want to install shell tab-completion for the vaultmind command.",
		short: "Generate the shell autocompletion script",
	},
	"vaultmind version": {
		group: groupSetup,
		when:  "you want the build version, commit, and date.",
		short: "Print the version, commit, and build date",
	},
	"vaultmind git": {
		group: groupSetup,
		when:  "you want git repository state relevant to VaultMind mutation policies.",
		short: "Inspect git repository state relevant to vault operations",
	},
	"vaultmind git status": {
		group: groupSetup,
		when:  "a script or agent needs to gate on the vault's branch, dirty, or merge/rebase state.",
		short: "Report git branch, dirty, and merge/rebase state for a vault",
	},
	"vaultmind experiment": {
		group: groupSetup,
		when:  "you want to inspect experiment tracking: retrieval quality, usage, traces, comparisons.",
		short: "Experiment tracking and reporting",
	},
	"vaultmind experiment report": {
		group: groupSetup,
		when:  "you want to measure retrieval quality — Hit@K and MRR per variant.",
		short: "Measure retrieval quality: Hit@K and MRR per variant",
	},
	"vaultmind experiment compare": {
		group: groupSetup,
		when:  "you want to see where retrieval variants disagree, without labeled ground truth.",
		short: "Surface where retrieval variants disagree, no labels needed",
	},
	"vaultmind experiment summary": {
		group: groupSetup,
		when:  "you want a memory-usage overview — top recalled notes and session-gap stats.",
		short: "Memory usage overview: top recalled notes, session gaps",
	},
	"vaultmind experiment trace": {
		group: groupSetup,
		when:  "you want to drill into one session's or note's retrieval history.",
		short: "Drill into a session's or note's retrieval history",
	},
	"vaultmind export": {
		group: groupSetup,
		when:  "you want a sanitized JSONL snapshot of experiment data to share with the VaultMind team.",
		short: "Export experiment data as a sanitized JSONL snapshot",
	},
	"vaultmind ping": {
		group: groupSetup,
		when:  "you want to smoke-test that the binary runs and renders output.",
		short: "Respond with a pong (connectivity smoke test)",
	},
}

// init runs last (zz_ filename) — after every command-registration init() — so
// decorateCommandCatalog walks a fully-assembled tree.
func init() {
	decorateCommandCatalog(RootCmd)
}

// decorateCommandCatalog stamps GroupID, the when-to-use annotation, the
// tightened Short, and the composed-into-Long "When to use:" line onto every
// non-hidden command that has a catalog entry. Hidden commands and commands
// without an entry are left untouched. Idempotent: composing the When line is
// guarded so repeated calls don't duplicate it.
//
// Cobra validates (and panics, via checkCommandGroups) that a child's GroupID
// is registered on its PARENT — not just on the root. So any command that has
// grouped children must itself register the four groups. We register them on
// every parent that ends up with a grouped child.
func decorateCommandCatalog(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if entry, ok := commandCatalog[sub.CommandPath()]; ok && !sub.Hidden {
				ensureCatalogGroup(c, entry.group)
				applyCatalogEntry(sub, entry)
			}
			walk(sub)
		}
	}
	walk(root)
}

// catalogGroupOrder is the ONE ordered list of the four catalog groups (id +
// title), in the order they render in `help`, `docs commands`, and the embedded
// onboarding doc. root.go registers the groups on RootCmd from this slice and
// catalogOptions feeds it to the commandcatalog renderers — one definition,
// every consumer (SSOT). catalogGroupTitle below is derived from it.
var catalogGroupOrder = []commandcatalog.Group{
	{ID: groupRetrieval, Title: groupRetrievalTitle},
	{ID: groupMaintenance, Title: groupMaintenanceTitle},
	{ID: groupLifecycle, Title: groupLifecycleTitle},
	{ID: groupSetup, Title: groupSetupTitle},
}

// catalogGroupTitle maps a group ID to its display title (SSOT for the title
// lookup used when registering a group on a parent), derived from the ordered
// list so a group is defined exactly once.
var catalogGroupTitle = func() map[string]string {
	m := make(map[string]string, len(catalogGroupOrder))
	for _, g := range catalogGroupOrder {
		m[g.ID] = g.Title
	}
	return m
}()

// catalogOptions is the SSOT bridge from cmd's catalog constants to the pure
// commandcatalog renderers: the ordered groups plus the when-annotation key.
func catalogOptions() commandcatalog.Options {
	return commandcatalog.Options{Groups: catalogGroupOrder, WhenKey: annotationWhen}
}

// buildCommandCatalog walks RootCmd into a structured Catalog using the cmd
// package's catalog SSOT. Both the terminal cheat-sheet (help) and the markdown
// reference (docs commands / onboarding embed) are rendered from this.
func buildCommandCatalog() commandcatalog.Catalog {
	return commandcatalog.Build(RootCmd, catalogOptions())
}

// commandsMarkdown renders the full grouped command reference as markdown — the
// body of COMMANDS.md, shared by `docs commands`, the onboarding embed
// generator, and the drift test (so the three can never diverge).
func commandsMarkdown() string {
	return commandcatalog.RenderMarkdown(buildCommandCatalog())
}

// ensureCatalogGroup registers groupID on cmd if not already present, so cmd's
// child carrying that GroupID passes cobra's group validation. Only the groups
// a parent actually uses are registered, avoiding empty group headers in help.
// Idempotent.
func ensureCatalogGroup(cmd *cobra.Command, groupID string) {
	for _, g := range cmd.Groups() {
		if g.ID == groupID {
			return
		}
	}
	cmd.AddGroup(&cobra.Group{ID: groupID, Title: catalogGroupTitle[groupID]})
}

// applyCatalogEntry writes one entry onto a command.
func applyCatalogEntry(c *cobra.Command, entry catalogEntry) {
	c.GroupID = entry.group
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations[annotationWhen] = entry.when
	if entry.short != "" {
		c.Short = entry.short
	}
	c.Long = composeWhenIntoLong(c.Long, c.Short, entry.when)
}

// composeWhenIntoLong returns Long with a "When to use:" line carrying the
// trigger phrase. When the command had no Long, the Short seeds it so --help
// still shows a body. The composition is idempotent — if the When line is
// already present, Long is returned unchanged.
func composeWhenIntoLong(long, short, when string) string {
	whenLine := fmt.Sprintf("%s %s", whenToUsePrefix, when)
	if strings.Contains(long, whenLine) {
		return long
	}
	body := long
	if strings.TrimSpace(body) == "" {
		body = short
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return whenLine
	}
	return body + "\n\n" + whenLine
}
