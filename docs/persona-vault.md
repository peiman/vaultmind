# A persona vault: an agent's identity across sessions

A knowledge vault holds what a project knows. A **persona vault** holds who an
agent is: the arcs (the moments that changed how it works), its principles, and
its episodes (captured sessions). The persona hooks load it at session start so
a new session continues as the same agent. See
**[building-an-identity-vault.md](building-an-identity-vault.md)** for how to
grow one from scratch — the arc method, and why an identity vault is
**personal** and usually shouldn't be committed to a shared repo.

```bash
vaultmind init ./agent-identity --profile persona
vaultmind hooks install <project-dir> --vault ./agent-identity --profile persona --merge

# Recall across several vaults (identity + a desk, say); the persona loads the first
vaultmind hooks install <project-dir> --vaults <identity-vault>,<desk-vault> --profile persona --merge
```

The persona profile adds the persona loader at session start and episode capture
at session end to the knowledge set. Every capture run, under Claude Code or
Codex, leaves a line in `~/.vaultmind/capture/capture.log` saying what happened.
Under Codex, episode capture runs when the session ends.

## Cold start — seed from your existing sessions

A new identity vault is empty, but you've probably worked with an agent for months. Point `episode capture` at a *directory* of past Claude Code transcripts to batch-capture them into episodes (recursive; empty/non-transcript files skipped), then surface candidate arcs — so the vault starts warm, not blank:

```bash
vaultmind episode capture ~/.claude/projects/<project> --output-dir ~/.vaultmind/persona/episodes
vaultmind arc candidates --vault ~/.vaultmind/persona
```

Codex sessions work too: each is one `rollout-*.jsonl` under `~/.codex/sessions/`. Codex keeps **every project's** sessions in that one folder, so capture the files of the project you mean rather than the whole folder, which would pull other projects' sessions into this vault. Codex's own sub-agent threads (its reviewers, spawned helpers) are passed over, the same as Claude Code's.

Subagent and workflow transcripts nested under a session are passed over: they carry the parent session's id, so capturing them would overwrite the session itself. The summary reports how many. (Pass one directly if you do want it captured.)

## The desk — where raw material lands

`arc candidates` reads two sources. `<vault>/episodes` holds session captures, which it phrase-matches for candidate moments — guesses worth checking. **The desk** is any note in the vault whose frontmatter says `type: journal`: something the agent stopped mid-session to write down, already judged worth keeping. Episodes are found; desk entries are chosen, and the report weights them accordingly.

```markdown
---
id: journal-2026-08-15-green-means-matches-my-assumption
type: journal
created: 2026-08-15
title: Green means "matches what I assumed", not "correct"
---
Spent an hour on a bug that turned out to be the test asserting the wrong thing.
The suite was green the whole time.
```

`type: journal` is what makes it a desk entry; the `id` is what lets you cite it later (from the arc it becomes, or via `note get`). An entry without one is still surfaced, flagged as unciteable.

Add `distilled_to: <arc-id>` to an entry once you've written its arc, and it stops being surfaced — so the list stays what's *pending*, not everything ever written. Each proposal is shown with the existing arcs it most resembles, which needs embeddings (`vaultmind index --embed`); without them the proposals still appear, minus the neighbours.

Keep the desk somewhere the agent owns outright. Use `--arcs-vault` when the desk and the arcs live in different vaults:

```bash
vaultmind arc candidates --vault ./agent-desk --arcs-vault ./agent-identity
```

## The example vault

VaultMind ships a small **fictional** persona vault — *Ada*, an agent that pair-programs with a developer named Sam on a toy CLI — with **concept cards** (`concepts/`) defining the vocabulary: arc, episode, principle, and how they link. From a repo checkout:

```bash
vaultmind index --vault examples/ada-vault
vaultmind index --embed --vault examples/ada-vault     # BGE-M3 on an ORT build, MiniLM otherwise
vaultmind ask "who are you" --vault examples/ada-vault
vaultmind ask "what did Ada learn about scope?" --vault examples/ada-vault
```
