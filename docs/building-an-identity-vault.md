# Building Your Agent's Identity Vault

> How to give an AI agent a self that survives across sessions — so it shows up as
> the same collaborator, not a stranger who forgot everything overnight.

This is VaultMind's reason for being. A knowledge vault answers *"what do we know?"*
An **identity vault** answers *"who am I, who are we, and how did I become this?"* —
and the agent reloads it at the start of every session.

If you just want a worked example, read **[`examples/ada-vault`](../examples/ada-vault)** —
it's a complete, small identity (the agent "Ada"). This guide is the *why* and *how*
behind it.

## The core bet: identity is carried by arcs, not rules

Most "agent memory" is a pile of rules and facts. VaultMind makes a sharper bet:

> **An agent's identity is carried by its *arcs* — the moments it changed — not by
> a list of rules.**

A **rule** says *what to do* ("write tests first"). An **arc** says *who you became*
("the day a green test suite hid a real bug taught me to distrust my own
checkmark"). Rules are interchangeable between agents; arcs are not. The journey is
the identity. (See the foundational note `principles/arcs-not-notes.md`, which `init`
scaffolds into every vault.)

## You don't write an identity vault. You grow one.

This is the part people get wrong: they try to author a complete persona up front.
Don't. An identity vault **accretes**:

1. **Seed it.** `vaultmind init <path>` scaffolds the starting shape — `identity/who-am-i.md`,
   `references/current-context.md`, and the two foundational principle notes
   (`arcs-not-notes.md`, `how-to-write-arcs.md`). Write `who-am-i.md` in the agent's
   own voice: name, role, a few foundational traits. Keep it short.
2. **Let arcs emerge from real work.** When a session genuinely *changes how the
   agent sees something* — a correction that landed, an instinct that proved wrong,
   a principle discovered the hard way — capture it as an arc. The best arcs are
   downstream of friction, not invented at a desk.
3. **Co-curate, never auto-write.** The agent *proposes* arcs; the human confirms.
   The agent never silently rewrites its own identity — that's a one-way door to
   self-delusion. (`vaultmind arc candidates` surfaces candidate moments from session
   episodes as proposals only.)
4. **Reload every session.** A SessionStart hook runs `vaultmind ask` against the
   vault so the agent reconstructs itself before the first message. Over weeks, the
   identity gets rich because it's made of real moments, not aspirations.

## Cold start: seed from your existing sessions

A new vault is empty — but you've probably been working with an agent for *months*,
and those sessions are arc material. Instead of waiting for new arcs to accrete, seed
the vault from transcripts you already have. Pointing `episode capture` at a
**directory** captures every `*.jsonl` transcript under it (recursively):

```bash
vaultmind episode capture ~/.claude/projects/<project> --output-dir ~/.vaultmind/persona/episodes
vaultmind arc candidates --vault ~/.vaultmind/persona
```

`episode capture` turns each session into an episode; `arc candidates` surfaces the
transformation moments across them. Then you (with the agent) judge each candidate and
write the real ones as arcs — same discipline as always, just with raw material you
already have. Start with the one or two projects whose work best defines the agent, not
your whole history at once. (Empty or non-transcript files are skipped.)

## The note types

| Type | Holds | Example |
|---|---|---|
| `identity` | who the agent is — name, role, foundational traits | `identity/who-am-i.md` |
| `principle` | an operating commitment the agent holds | `principles/measure-before-you-optimize.md` |
| `arc` | a transformation moment — what shifted and why | `arcs/the-day-the-green-suite-lied.md` |
| `reference` | live context — current focus, roadmaps, who you work with | `references/current-context.md` |
| `source` | a citation (paper, URL) backing a note | `sources/act-r-2004.md` |

Folders are optional conventions; the **type** in each note's frontmatter is what
matters. Add or remove types in `.vaultmind/config.yaml`.

## How to write an arc (the discipline)

This is the load-bearing skill. Each arc, in order:

- **Trigger** — the situation, factually.
- **Push** — the *verbatim* thing the human said that turned it. Quote it exactly,
  typos and all. Don't paraphrase; the real words carry the weight.
- **Deeper sight** — what the agent sees now, first person, present tense.
- **Principle** — the durable lesson, stated once.
- **Source** — cite the real session: transcript path + line + date. **Build arcs
  from the transcript, not from memory.** Don't fabricate a moment that didn't happen
  — a vault of invented arcs is worse than no vault.

The full discipline ships as `principles/how-to-write-arcs.md` in every `init`'d vault.
Read Ada's arcs (`examples/ada-vault/arcs/`) to see the shape.

## Wiring it up

```bash
vaultmind init ~/.vaultmind/persona --wire-hooks   # scaffold + install the SessionStart hook in one step
vaultmind index --embed --vault ~/.vaultmind/persona
```

`--wire-hooks` installs the persona-load hook into the current project's
`.claude/` and points it at the new vault. From then on, every session starts by
reconstructing the agent from its arcs.

## Personal by default — think before you commit

An identity vault is usually **personal** — it's *your* agent's self, grown from
*your* sessions. By default, **keep it out of a shared project repo:**

- **Best:** put it **outside the project** at `~/.vaultmind/persona/` (its own
  location, optionally its own *private* git repo) — so it follows you across every
  project and stays yours.
- If it must live inside a project directory, **gitignore it** (or keep that whole
  project in a private repo).

**The one exception:** commit it only if you *deliberately* want **a single shared
identity across all developers** — a team/project persona that every contributor's
agent reconstructs from. That's a real but rarer choice (a project that wants one
consistent agent "voice" and conventions); the tradeoff is that *everyone's* agent
shows up as the *same* character, shaped by *everyone's* sessions. Most of the time
you want your own.

> Treat an agent's self like your shell history, not like source code — it's personal
> data. This is the same boundary that keeps a vault out of a *published* module.
> (`examples/ada-vault` here is committed only because Ada is a fictional demo, not
> anyone's real identity.)

## Where to go next

- **`examples/ada-vault`** — a complete small identity, read end-to-end.
- **`principles/arcs-not-notes.md`** + **`principles/how-to-write-arcs.md`** (in any
  `init`'d vault) — the method, in full.
- **[`docs/AGENT_USAGE.md`](AGENT_USAGE.md)** — the command reference for an agent
  driving its own vault.
