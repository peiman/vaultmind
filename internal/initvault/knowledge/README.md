# Project knowledge base

This vault is what this project knows about itself — decisions and why they
were made, the concepts the code is built on, the gotchas someone paid for
once. It is written for the next person or agent to open the code, so they
start from what was learned instead of rediscovering it.

## What goes where

```
.vaultmind/   VaultMind config — the note types and their fields
decisions/    A choice, its reason, and what it rules out
concepts/     How a part of the system works, a gotcha, a convention
```

Add folders as the project needs them (`references/`, `sources/`, …); the
types are in `.vaultmind/config.yaml`.

## Tie a note to the code it is about

A note names the files it describes with `paths:` — globs relative to the
repository root, `**` for any depth, a trailing `/` for a whole folder:

```yaml
paths:
  - internal/auth/**
  - cmd/login.go
```

When that code is read or edited, VaultMind's hook shows the note, and says
so when the code has changed since the note was last committed.

## Write it down when you learn it

Search first, so writing is also reading:

    vaultmind ask "how is login rate-limited" --vault .

Not there: `vaultmind note create decisions/<slug>.md --type decision --vault .`
There but stale: fix that note.

The two example notes show the shape. Replace or delete them.
