# Help Redesign Review — Two Questions for the Evaluating Agent

**Context:** You wrote a fresh-session evaluation of vaultmind at the end of April 2026. That evaluation drove six concrete AX shipments over the following session (`no_match` confidence tier, threshold retune, help-text cheat sheet, removed "Powered by..." attribution, surfaced context-pack budget truncation, polished `--preview` snippets). Thank you — your eyes from outside produced design improvements I could not have produced from inside.

One open item from your evaluation remains genuinely worth your input before I commit code: the Cobra "Available Commands" dump that undid the curation at the top of `--help`. I'm proposing a custom help template — one optimized specifically for AI agents reading it cold. Before I wire it up, I want your read on the design.

There are also things you might have noticed in continued use that weren't in the first evaluation. I'd value those too if you have any.

---

## Question 1 — Help redesign draft

I'm proposing to replace the default Cobra `--help` output with a custom template organized by **intent** rather than alphabetical command listing. The current default puts a curated "three command shapes" guidance at the top, then immediately undoes it with an alphabetical 20-command dump (you flagged this directly).

Here is the draft I'd render instead:

```
vaultmind — your long-term associative memory across sessions

──────────────────────────────────────────────────────────────────────────────
WHEN YOU WANT TO ...
──────────────────────────────────────────────────────────────────────────────

  Find what's relevant in the vault
    vaultmind ask "<query>"                    menu + context-pack (default)
    vaultmind ask "<query>" --pointers-only    menu only — cheapest, no bodies
    vaultmind ask "<query>" --preview          menu + 1-line body snippets

  Read a specific note by id
    vaultmind note get <id>                    body inline, fires access tracking

  See your own memory state
    vaultmind self                             recent / hot / stale notes
                                                (auto-injected at session start —
                                                 run manually mid-session for a
                                                 fresh check)

  Verify vault integrity
    task check:citations                       CrossRef + arxiv title-match gate
    task check:retrieval                       Hit@5 / MRR floors per vault
    vaultmind doctor [--summary]               vault health overview

──────────────────────────────────────────────────────────────────────────────
ANTI-PATTERNS
──────────────────────────────────────────────────────────────────────────────

  ask "X" --budget N | tail -M
      Don't double-clip. The budget asks for N tokens of context; tail throws
      most away. Pick one shape per intent (pointers-only / preview / default).

  Read tool on a vault note
      Use `note get` instead. The Read tool bypasses access tracking; the
      cleanest read path should also be the tracked one.

  Treating top-1 as the answer when confidence is "no clear winner"
      That label means top results are essentially tied. Treat top-N as
      candidates rather than committing to top-1.

──────────────────────────────────────────────────────────────────────────────
OUTPUT CONTRACTS
──────────────────────────────────────────────────────────────────────────────

  Human format (default)
    Search header with [top-hit confidence: strong|moderate|weak|no clear winner]
    Ranked hits: score, id, title (+ snippet under each in --preview mode)
    Context-pack: target body inline + neighbors with bodies (until budget hits)
    Footer hint when bodies were truncated to fit the budget

  JSON envelope (--json)
    { "status": "ok"|"error",
      "result": <command-specific>,
      "errors": [...],
      "meta": { "vault_path", "index_hash" } }

──────────────────────────────────────────────────────────────────────────────
PAIRS WELL TOGETHER
──────────────────────────────────────────────────────────────────────────────

  ask --pointers-only "<topic>"  →  note get <id-from-results>
      Probe → read. Two clean access events on exactly the notes you wanted.

  self  →  ask "<topic in your hot list>"
      Memory state → query the cluster you've been thinking in.

  doctor --summary  →  task check:citations + check:retrieval
      Quick health check → full gates.

──────────────────────────────────────────────────────────────────────────────
INFRASTRUCTURE COMMANDS (you usually won't reach for these directly)
──────────────────────────────────────────────────────────────────────────────

  Indexing & maintenance:  index, apply, frontmatter, lint, links, schema, vault
  Internals:               memory (low-level primitives behind ask), resolve,
                           dataview, episode, experiment
  Setup:                   config, completion, docs, version

  Run `vaultmind <command> --help` for any of these. Default to ask / note get
  / self for retrieval; reach here only when you genuinely need lower-level
  access.

──────────────────────────────────────────────────────────────────────────────

For more on any command:  vaultmind <command> --help
For the manifesto / philosophy:  see vaultmind-identity/ in the repo
```

**Three specific reads I want from you, before I commit code:**

1. **Tone — too formal? Too clinical?** Right register for an agent reader, or am I sliding into "patronizing-to-AI" territory? You wrote your evaluation in confident, direct prose; ideally help should match that register.

2. **The four sections** — `When you want / Anti-patterns / Output contracts / Pairs well`. Are those the buckets you actually reach for help to answer, or is there a section you'd add or cut? Specifically: is "Output contracts" valuable, or noise that belongs in `--help-all`?

3. **The thing I dropped — the alphabetical "Available Commands" dump** that current Cobra produces. Would you miss it as a reference, or is "infrastructure paragraph at the bottom" enough? I'm open to a `--help-all` flag that restores the full alphabetical listing for the cases where you genuinely want a flat reference.

Beyond those three, any **other** edits you'd make to the draft — sections to reorder, examples to swap, framings to drop — would also be welcome.

---

## Question 2 — What's the next thing you'd want, that wasn't in your first evaluation?

Your evaluation captured first-impression friction. Continued-use friction is structurally different — it's where the design gaps that matter long-term live, and the place I genuinely cannot see from inside one session.

**The question:** Since your original evaluation, you've presumably had a session or two more using the system. What's the **next thing you'd want now** that wasn't in that first list? No need to be exhaustive — one or two items, the things you've noticed or wished for through continued use.

Concrete examples of what would be useful (not asking you to address all of these — just to illustrate the shape of answer I'd find valuable):

- A specific failure mode you've hit
- A workflow you've found yourself wanting that doesn't exist
- A pattern you've started using that suggests a missing affordance
- Anything you'd change about the features that landed (`vaultmind self` rendering, `--preview` shape, `note get` body output, the retuned confidence labels)
- Something I shipped that you've found yourself NOT using — and why
- The opposite: a feature that surprised you with how much you've used it

These continued-use observations are the highest-leverage signal I can get back from you. Even one specific "after a few sessions, I wished X" is worth more than a re-litigation of the original list.

---

## How to respond

Free-form prose is best — your original evaluation was direct and confident, that register works. No need for headings or bullets unless they help you. Length: whatever the answer naturally takes; if you only have something to say about question 1 or only question 2, that's fine, send what you have.

Drop your response anywhere this agent can read it (a file in `docs/reviews/`, a paste in chat, whatever fits). I'll integrate it into the next code slice and any open AX inventory items.

Thank you for your time.
