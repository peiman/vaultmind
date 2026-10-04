# VaultMind

**A project's knowledge, where its agents can use it.**

A codebase records what the system does. It rarely records why: the decisions and what they ruled out, how the parts work, the gotchas someone already paid for. VaultMind keeps that knowledge in a vault of plain Markdown notes in Git, and gets it to the agents that work on the project: searched, ranked and delivered into their context as they work, with no server and no LLM of its own.

It's a single Go binary. It builds a derived index (full-text, dense, sparse and late-interaction embeddings) and a graph of the `[[wikilinks]]` between notes. It needs the embedding model it downloads on first use (BGE-M3, about 2.3 GB, for the full hybrid; MiniLM, about 90 MB, for the pure-Go build), and `bash` and `python3` for the agent hooks.

## What you can do

**Get knowledge in**

| | |
|---|---|
| Start a vault | `vaultmind init ./knowledge` |
| Import a folder of docs (Markdown, PDF, Word, PowerPoint, Excel, HTML, CSV, EPUB, images) | `vaultmind import ./docs --vault ./knowledge` |
| Import a web page, or a whole docs site | `vaultmind import https://docs.example.com --vault ./knowledge --crawl` |
| Keep the vault in step with a folder or site as it changes | `vaultmind import ./docs --vault ./knowledge --watch` |
| Import a zip or tar of docs | `vaultmind import ./docs.zip --vault ./knowledge` |
| Write a note | `vaultmind note create decisions/cache.md --type decision --field title="Cache in front of the API" --field status=accepted --vault ./knowledge` |

A re-import brings changes across and keeps notes you edited by hand (`--force` overwrites them). `--dry-run` shows what an import would do. A long document becomes an index note plus one note per section. Images become notes of their metadata, their text (OCR) and, with `--vision-endpoint`, a description from a vision model.

**Find it**

| | |
|---|---|
| Ask a question; get the ranked notes and their most relevant text | `vaultmind ask "why sqlite and not postgres?" --vault ./knowledge` |
| Search without bodies (`--mode keyword`, `semantic` or `hybrid`) | `vaultmind search "retries" --vault ./knowledge --mode hybrid` |
| See what the vault covers: folders, what each is about, one line per note | `vaultmind tree --vault ./knowledge --depth 1` |
| Find the notes about a code file (via `paths:` in their frontmatter) | `vaultmind tree --vault ./knowledge --for internal/auth/token.go` |
| Read a note | `vaultmind note get decision-sqlite --vault ./knowledge` |
| See what links to a note, and what it links to | `vaultmind memory links decision-sqlite --in --vault ./knowledge` |
| Ask across several vaults at once | `vaultmind ask "..." --vaults ./knowledge,./team-notes` |

**Give it to agents**

| | |
|---|---|
| Claude Code: recall on every prompt, the notes about a file as it's read, related notes before a commit | `vaultmind hooks install . --vault ./knowledge --merge` |
| Codex CLI | `vaultmind hooks install . --vault ./knowledge --agent codex --merge` |
| Cursor | `vaultmind hooks install . --vault ./knowledge --agent cursor --merge` |
| Any MCP client, including Claude Desktop and agents without a shell | `vaultmind mcp --vault ./knowledge` |
| Check that the hooks are installed and wired | `vaultmind hooks status .` |

**Keep it healthy**

| | |
|---|---|
| Check the vault: unresolved and dead links, missing required fields, notes not yet embedded, a stale index, drifted hooks | `vaultmind doctor --vault ./knowledge` |
| Fix what can be fixed automatically | `vaultmind doctor heal --vault ./knowledge` |
| Re-index after editing notes by hand (`--embed` refreshes embeddings) | `vaultmind index --embed --vault ./knowledge` |

Every command has `--help`, and `--json` for scripts. `vaultmind --help` lists all of them by what you want to do.

## Install

**Quick — MiniLM, every platform:**

```bash
go install github.com/peiman/vaultmind@latest   # requires Go >= 1.26.6
```

A pure-Go binary: full-text + MiniLM dense retrieval (2 lanes). Ideal for trying VaultMind — it does **not** include BGE-M3's sparse + ColBERT lanes.

**Full 4-way hybrid — BGE-M3 (recommended for real use):**

Download the self-contained ORT archive for your platform (`darwin-arm64`, `linux-amd64`) from the [releases page](https://github.com/peiman/vaultmind/releases) — `vaultmind_<version>_<os>_<arch>_ort.tar.gz` — extract, and run. The official `libonnxruntime` is bundled (found automatically next to the binary), so there's nothing else to install — **no compiler required.** Upgrading an existing install? Extract elsewhere and `mv` the new binary into place rather than copying over the old one — `mv` is atomic and gives a fresh inode. Overwriting in place has been seen once on macOS to trip a codesigning SIGKILL; the cause is unconfirmed, but replacing rather than overwriting avoids it and is the safer form anyway.

> **Which one?** For a **small vault or a slow machine, MiniLM is genuinely fine** — its dense lane covers a small corpus well, and its lighter query encoder keeps per-prompt latency low (that cost lands on *every* recall). Reach for **BGE-M3 when your vault is large and varied** and recall quality matters most. Note: `go install` is the *only* path that can't produce BGE-M3 (cgo can't travel through it) — so if your setup uses `go install`, you're on MiniLM by design; the prebuilt archive above is the no-compile way to the full hybrid.

**From source (any platform with a C toolchain):**

```bash
git clone https://github.com/peiman/vaultmind && cd vaultmind
brew install onnxruntime   # macOS; Linux: install from the ONNX Runtime releases
task setup:ort             # downloads the tokenizer static lib
task build                 # auto-selects ORT when the tokenizer lib is present
```

See **[docs/embedding-backends.md](docs/embedding-backends.md)** for every backend, platform, performance note, and the dense-only vs. 4-way-hybrid tradeoff.

## Quickstart

```bash
# 1. start a vault and bring in the docs a project already has
vaultmind init ./knowledge
vaultmind import ./docs --vault ./knowledge --dry-run   # see what it would do
vaultmind import ./docs --vault ./knowledge             # re-run after the docs change

# 2. embed it (import indexes; this adds meaning-based search)
vaultmind index --embed --vault ./knowledge

# 3. ask it, and see what it holds
vaultmind ask "what did we decide about retries?" --vault ./knowledge
vaultmind tree --vault ./knowledge --depth 1

# 4. hand it to your agent
vaultmind hooks install . --vault ./knowledge --merge
```

`init` writes a type registry, a README for the vault and starter notes. Add your own notes as Markdown with frontmatter; `vaultmind index` picks up hand edits.

## Agents: hooks and MCP

**Hooks deliver the vault without being asked.** `hooks install` writes scripts into the project and wires them into the agent's settings (`--merge` adds to existing hooks and never removes them; `--dry-run` previews). A knowledge vault gets: the vault map and health at session start (and again after `/clear` and compaction), the notes that match each prompt as short excerpts, the notes about a code file when the agent reads or edits it, and related notes before a commit, push or merge. Without `--vault`, the hooks look for `<project-dir>/vaultmind-identity`.

- **Codex needs your approval, and skips the hooks silently without it.** Trust the project when Codex asks, then run `/hooks` inside Codex and trust the VaultMind hooks, and again after every upgrade. `vaultmind hooks status <project-dir>` checks each hook the way Codex does and fails while any would be skipped. Codex projects keep the scripts in `.vaultmind/scripts/`. Read-tracking, the code map and the pre-compaction prompt are Claude Code only.
- **Cursor adds context only at session start and after a tool runs.** So under Cursor the agent gets the vault map at session start, the notes about a file after it reads or edits that file, and related notes after a commit, but no per-prompt recall: Cursor's hooks cannot add context to a prompt. The session-start message tells the agent to ask the vault itself, and in our tests it did.

`vaultmind hooks status <project-dir>` reports both halves: whether each script matches the copy in the binary, and whether each event is actually wired. See **[docs/AGENT_USAGE.md](docs/AGENT_USAGE.md)** for the day-to-day agent workflow.

**MCP gives any client the tools.** `vaultmind mcp` serves a vault over stdio to Claude Desktop, Cursor or any MCP client. Its tools are `ask`, `search`, `note_get`, `tree`, `links`, `note_create` and `import` (a web page or site: http(s) URLs from public addresses only, since a page the agent read could prompt it). Each one runs the vaultmind command of the same name with `--json`, so a tool answers exactly what the CLI answers. The vaults are fixed when the server starts, so a client can't point a tool at another directory.

```bash
claude mcp add vaultmind -- vaultmind mcp --vault /path/to/vault
```

Claude Desktop, Cursor (`.cursor/mcp.json`) and most other clients take the same entry: `{"mcpServers": {"vaultmind": {"command": "vaultmind", "args": ["mcp", "--vault", "/path/to/vault"]}}}`. MCP gives an agent the tools, while the hooks deliver the vault without being asked, so use both where your agent has hooks.

## An agent's own identity (persona vaults)

A vault can also hold who an agent is: the moments that changed how it works, its principles, and its captured sessions. Loaded at session start, it lets a new session continue as the same agent. `vaultmind init --profile persona` makes one, and `hooks install --profile persona` loads it. See **[docs/persona-vault.md](docs/persona-vault.md)** for the persona hooks, seeding a vault from past sessions, the desk, and the example vault, and **[docs/building-an-identity-vault.md](docs/building-an-identity-vault.md)** for the arc method.

## How it works

Notes are the source of truth; the SQLite index is derived from them and rebuilt by `vaultmind index`. Search fuses full-text, dense, sparse and ColBERT lanes, and reports how far the top hit clears the vault's own off-topic noise floor, so "nothing relevant" is an honest answer. A hit brings along the notes it links to. See **[docs/how-it-works.md](docs/how-it-works.md)** for retrieval, context packs, excerpts and delivery.

## The local usage log

VaultMind keeps a local SQLite log of retrieval events — which queries surfaced which notes, and which notes an agent then opened — as the raw material for learning from use. Today it is a record, not an input: it does not change what search returns. It is on by default, and it is worth knowing exactly what that means.

**On your disk, by default:** the **full query text**, the vault path, the note ids returned, and caller metadata (`$USER`, hostname, `CLAUDE_PROJECT_DIR`). The file is created `0600` — together with the `-wal` and `-shm` sidecars SQLite writes beside it, which hold the same material — at

- **macOS:** `~/Library/Application Support/vaultmind/experiments.db`
- **Linux:** `$XDG_DATA_HOME/vaultmind/experiments.db` (default `~/.local/share/vaultmind/experiments.db`)

Nothing is transmitted — VaultMind has no uploader, and `doctor`'s once-a-day version check is the only network call it makes on its own (see "The one network call").

**`experiments.telemetry` chooses what leaves**, not what is written:

| | writes locally | `vaultmind export` emits |
|---|---|---|
| `anonymous` *(default)* | everything above | query text, vault path, note ids and paths, and caller metadata **stripped** |
| `full` | everything above | everything |
| `off` | **nothing** — the log is never created | nothing to export |

So `anonymous` means *anonymous when shared*. It is not a redaction at write time, and a reader who opens the database will find their queries in it. Reading an existing log still works under `off`; turning it off is a decision about new data, not a lock on what you already have.

**To turn it off** — in `~/.config/vaultmind/config.yaml` (or `$XDG_CONFIG_HOME/vaultmind/config.yaml`):

```yaml
experiments:
  telemetry: off
```

The nesting matters. A bare `experiments: off` does **not** work — it is read as an empty value and you get the `anonymous` default, which is the opposite of what you asked for. `VAULTMIND_EXPERIMENTS_TELEMETRY=off` works too, and `vaultmind config` prints what is currently in effect.

Turning it off loses that history; it changes nothing about today's search results.

Sharing remains something you do on purpose: there is no automatic upload, and `export` only writes a file.

## The one network call

`vaultmind doctor` asks the Go module proxy once a day whether a newer VaultMind exists, and prints a line if so. That is the only time VaultMind reaches the network on its own, and it sends nothing about you or your vault — it is a `GET` for a version number, the same request `go install …@latest` makes. The answer is cached for 24 hours, times out in 3 seconds, and is silent on any failure.

It lives on `doctor` and nowhere else: that is the command you run to ask whether your setup is healthy, so a network call there is expected rather than a surprise — and putting it on `ask` would tax every query. Set `VAULTMIND_NO_UPDATE_CHECK=1` to opt out; the check returns before making any request, and the notice itself tells you that variable exists. (`import` reaches the network when you give it a URL, and `--vision-endpoint` sends images to the endpoint you name; neither happens on its own.)

## Contributing

VaultMind is a Go CLI built on the [ckeletin-go](https://github.com/peiman/ckeletin-go) scaffold (the `.ckeletin/` framework layer). `task check` is the single quality gate — it runs formatting, linting, architecture and security checks, the full test suite, and the coverage floor. If it passes, the change is sound regardless of who wrote it.

- Read **[AGENTS.md](AGENTS.md)** for architecture rules and conventions, and **[CONTRIBUTING.md](CONTRIBUTING.md)** before opening a PR.
- TDD: write the failing test first; commit test + implementation together.

## License

See **[LICENSE](LICENSE)**.
