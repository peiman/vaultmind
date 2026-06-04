# Embedding Backends — Adoption Guide

> Which backend VaultMind uses to turn your vault into embeddings, how to choose one, and the tradeoffs that decide retrieval quality. The central fact: retrieval quality is fixed at **index** time, and only the BGE-M3 backends produce VaultMind's full **4-way RRF hybrid** (dense + sparse + ColBERT). Everything else is dense-only.
>
> **Last verified: 2026-06-03** against the working tree. The CoreML and Ollama sections include results from live experiments run on this date (Apple Silicon, libonnxruntime 1.25.0, Ollama v0.24.0).

## TL;DR — which backend should I use?

VaultMind embeds your vault with one of several backends. The single most important fact: **only the BGE-M3 backends (ORT-CPU, ORT+CoreML, MPS sidecar) produce the full 4-way RRF hybrid** (dense + sparse/lexical + ColBERT/late-interaction, fused with reciprocal-rank fusion). Every other backend — pure-Go MiniLM, or a hypothetical Ollama backend — gives you **dense-only retrieval**, which is strictly weaker for ranking quality. This tradeoff drives every recommendation below.

| Your situation | Use this backend | What you get |
|---|---|---|
| macOS arm64 / Linux amd64, want best retrieval, **no build** | **Prebuilt ORT archive** — download `vaultmind_<ver>_<os>_<arch>_ort.tar.gz`, extract, run | Full 4-way BGE-M3 hybrid (libonnxruntime bundled) |
| macOS x86_64 / Linux arm64 (or building from source) | **ORT-CPU** (`task setup:ort && task build`) | Full 4-way BGE-M3 hybrid |
| Apple Silicon, indexing a large/often-changing vault | **MPS Python sidecar** (on top of an ORT build) | Full 4-way hybrid, ~4× faster indexing |
| Windows | **Pure-Go MiniLM** (no fast path exists) | Dense-only MiniLM (384-dim) |
| Want zero build friction, accept lower retrieval quality | **Ollama** *(not yet implemented)* / Pure-Go MiniLM | Dense-only (1024-dim BGE-M3 dense via Ollama, or 384-dim MiniLM) |
| Just want to query an already-indexed vault | Any build (query is ~1s on all) | Quality is fixed at **index** time, not query time |

**Decision tree:**

```
Do you need VaultMind's best retrieval (sparse + ColBERT, not just dense)?
├── NO  → Pure-Go MiniLM (task build:fast). Zero system deps, single binary.
│         (Or Ollama dense-only once implemented — easiest to stand up, 1024-dim.)
└── YES → You need a BGE-M3 backend (full 4-way hybrid):
          ├── Windows?            → No fast path today. Stuck on pure-Go MiniLM. (gap)
          ├── Apple Silicon + big/frequent reindex? → MPS sidecar (Python+torch) on an ORT build.
          ├── macOS arm64 / Linux amd64? → Prebuilt ORT archive (download *_ort.tar.gz — no build). The default.
          └── macOS x86_64 / Linux arm64 → ORT-CPU from source (task setup:ort && task build).

Apple Silicon GPU in-process (CoreML)? → BLOCKED today (issue #34). Do not rely on it.
```

> **Key gotcha — the silent downgrade.** `task build` with no `lib/libtokenizers.a` present produces a pure-Go binary whose `DefaultModel()` returns `minilm`, not `bge-m3`. You get a working tool that is *quietly dense-only*, losing the entire 4-way hybrid. The only signal is one ⚠ line at build time and a post-hoc `doctor` warning. If you want the hybrid, you must run `task setup:ort` **first**.

## The backends

### Pure-Go hugot (MiniLM)

- **What it is:** The default, zero-dependency backend. Compiled from `session_go.go` (`//go:build !cgo || !ORT`), where `newBGEM3Session()` calls `hugot.NewGoSession()`. `BackendName()` returns `"go"`, which makes `DefaultModel()` resolve to `"minilm"`.
- **System deps:** None. Single static Go binary, no CGO, no native libraries.
- **Setup steps:**
  1. Build: `task build:fast` (always pure-Go) or `task build` (falls back to pure-Go with a ⚠ warning when `lib/libtokenizers.a` is absent).
  2. Index: `vaultmind index --vault <vault> --embed` (auto-selects `minilm` via `DefaultModel()`).
  3. Query: `vaultmind ask/search --vault <vault>` works on MiniLM dense vectors.
- **Platforms:** All — macOS arm64, macOS x86_64, Linux x86_64/arm64, Windows. Pure Go, no native libs.
- **Perf:** MiniLM query embedding ~1s for short text (fine). **BGE-M3 indexing here is "hours for ~130 notes"** and is hard-blocked: `cmd/index.go`'s `guardBGEM3SlowBackend` refuses `--model bge-m3` on this backend unless you pass `--allow-slow-backend`.
- **Retrieval quality:** **Dense-only, MiniLM 384-dim. NOT the 4-way hybrid** — no sparse, no ColBERT. This is the weakest retrieval tier.
- **How to select it:** Build without `-tags ORT` (the default when `libtokenizers.a` is missing). To force BGE-M3 anyway: `--model bge-m3 --allow-slow-backend` (and accept hours of indexing).
- **Status:** Shipped.

### ORT in-process (CPU execution provider)

- **What it is:** The intended **default fast indexing path** and the only out-of-box source of the full 4-way BGE-M3 hybrid. Compiled from `session_ort.go` (`//go:build cgo && ORT`), where `newBGEM3Session()` calls `hugot.NewORTSession(...)`. `BackendName()` returns `"ort"`, so `DefaultModel()` resolves to `"bge-m3"`.
- **System deps:** `libonnxruntime.{dylib,so}` on the system (`brew install onnxruntime` on macOS; package/release on Linux) **plus** project-local `lib/libtokenizers.a` (downloaded by `setup-ort.sh`) **plus** a CGO toolchain (C compiler). Pinned stack: libonnxruntime 1.25.0, hugot v0.7.0, onnxruntime_go v1.30.1.
- **Setup steps:**
  1. One-time deps: `task setup:ort` (downloads `libtokenizers.a`; requires `libonnxruntime` already present — install via `brew install onnxruntime` first).
  2. Build onto PATH: `task install` (auto-selects ORT when `lib/libtokenizers.a` exists) or throwaway `task build:ort` → `/tmp/vaultmind-ort`.
  3. Index: `vaultmind index --vault <vault> --embed` (auto-selects `bge-m3`).
  4. Verify: `vaultmind doctor --vault <vault>`.
- **Platforms:** `setup-ort.sh` supports Darwin/arm64, Darwin/x86_64, Linux/amd64, Linux/aarch64, Linux/arm64. **No Windows path** — the script has no Windows branch and no libtokenizers download for it.
- **Perf:** Accelerates BGE-M3 indexing from "hours" to **a few minutes** for ~130 notes (design estimate ~2–3 min; not formally benchmarked) — vs *hours* on pure-Go. Runs on the CPU execution provider (`Acceleration()` → `"ort+cpu"`). First `index --embed --model bge-m3` triggers a **~2.2GB model download** from HuggingFace.
- **Retrieval quality:** **Full BGE-M3 4-way hybrid** — dense (1024) + sparse (lexical) + ColBERT (multi-vector), RRF-fused. This is the reference quality path. Implements `FullEmbedder` via `EmbedFullBatch`.
- **How to select it:** Build with `-tags ORT` + CGO (auto when `lib/libtokenizers.a` is present). `DefaultModel()` then auto-picks `bge-m3`.
- **Status:** Shipped.

### ORT + CoreML execution provider (Apple Silicon GPU/ANE) — BLOCKED

- **What it is:** The same ORT build, with the CoreML execution provider wired in as a **runtime opt-in**: `VAULTMIND_ENABLE_COREML=1` AND `GOOS==darwin && GOARCH==arm64` (`shouldEnableCoreML`). When enabled, `newBGEM3Session()` adds `options.WithCoreML(...)` and `Acceleration()` returns `"ort+coreml"`. `BackendName()` stays `"ort"`.
- **System deps:** Same as ORT-CPU (libonnxruntime 1.25.0 + libtokenizers.a + CGO) plus macOS CoreML frameworks. No extra adopter install — but **it does not work**.
- **Setup steps:** Not adoptable today. The wiring is kept solely so a post-fix retest is one env var: `VAULTMIND_ENABLE_COREML=1` on an ORT build on darwin/arm64. Expect session-creation failure (see CoreML status section).
- **Platforms:** macOS arm64 only (gated by GOOS/GOARCH; no-op elsewhere).
- **Perf:** Intended to use Apple Silicon GPU/ANE. **Not measurable** — fails before inference.
- **Retrieval quality:** Would be the full 4-way hybrid if it worked (same code path as ORT-CPU). Currently produces nothing.
- **How to select it:** `VAULTMIND_ENABLE_COREML=1` on an ORT/darwin-arm64 build.
- **Status:** **Blocked** (issue #34).

### Python + PyTorch + MPS sidecar (Apple Silicon GPU) — the only working Mac GPU path

- **What it is:** A Python subprocess behind a JSON-line contract that runs BGE-M3 on Apple's MPS GPU. **Indexing-only.** Selected at runtime inside `RunEmbed` when `model == "bge-m3"` AND `VAULTMIND_USE_SIDECAR` is set; on any startup failure it logs one WARN and falls back to the in-process embedder.
- **System deps:** External Python 3.11/3.12/3.13 with `torch` + `transformers` installed, and `torch.backends.mps.is_available() == true` for GPU. (Note: torch is **not** installed by default — verified absent even on the dev box.) The Go binary can be any build, but is paired with an ORT build so the in-process fallback is also fast. First run downloads BGE-M3 (~2GB) into the HF cache.
- **Setup steps:**
  1. One-time venv: `python3 -m venv ~/.vaultmind/sidecar-venv && pip install torch transformers`.
  2. Export all three env vars before indexing: `VAULTMIND_USE_SIDECAR=1`, `VAULTMIND_SIDECAR_PYTHON=<venv>/bin/python`, `VAULTMIND_SIDECAR_SCRIPT=<abs path>/internal/embedding/sidecar/embed_server.py`. (The in-code default script path is relative and labeled "for tests" — an installed binary cannot find the script without an **absolute** path you supply.)
  3. Run `vaultmind index --vault <vault> --embed --model bge-m3`; look for the log line `BGE-M3 sidecar active for indexing device=mps`.
  4. If anything is misconfigured, it silently falls back to in-process ORT+CPU (one WARN line, easy to miss in `--json` runs).
- **Platforms:** GPU path is Apple Silicon (MPS) only. On non-MPS hosts the sidecar still runs but selects `device="cpu"` — no GPU benefit, just an out-of-process CPU path.
- **Perf:** "Empirically ~4× faster than in-process ORT+CPU" for BGE-M3 indexing, with a ~4s one-time model-load/startup that amortizes across the pass. (Inputs are forwarded as per-text singleton forward passes for correctness — batched MPS attention masking diverges; cost is only ~1.04× vs batched.)
- **Retrieval quality:** **Full BGE-M3 4-way hybrid.** Implements `FullEmbedder` (`EmbedFullBatch` returns dense + sparse + ColBERT), with heads running inside the sidecar on MPS so per-modality tensors stay on GPU. Sidecar-embedded vaults produce the same rankings as in-process; mixed-state vaults work.
- **How to select it:** Set the three env vars above on a build (preferably ORT) at index time.
- **Status:** Experimental.

### Ollama (dense-only) — EVALUATED, not yet implemented

- **What it is:** A candidate "easy-mode" backend. Ollama runs a local daemon on `:11434`; `ollama pull bge-m3` fetches the same BAAI/bge-m3 weights (GGUF F16, ~1.2GB). VaultMind would wire an `OllamaEmbedder` implementing the `Embedder` interface (`Embed`/`EmbedBatch` → `POST /api/embed`, `Dims()=1024`). **No such backend exists in the code today** — a grep for "ollama" across `internal/` and `cmd/` returns nothing.
- **System deps:** Ollama daemon (no CGO, no libonnxruntime, no libtokenizers, no `-tags ORT`, no Python). Pin the integration to the modern `POST /api/embed` endpoint — **not** the legacy `/api/embeddings`.
- **Setup steps:**
  1. Install Ollama (macOS: app or `brew install ollama`; Linux: `curl -fsSL https://ollama.com/install.sh | sh`; Windows: native installer). Daemon auto-runs on `:11434`.
  2. `ollama pull bge-m3` (~1.2GB F16; needs ~1GB free VRAM/RAM).
  3. Smoke-test: `curl http://localhost:11434/api/embed -d '{"model":"bge-m3","input":"hello"}'` and confirm `embeddings[0]` has length 1024.
  4. (Once implemented) point VaultMind at `OLLAMA_HOST` and model `bge-m3`, then `vaultmind index --embed` and `vaultmind ask`.
- **Platforms:** **Uniform across all** — macOS arm64/x86_64, Linux, Windows — with one `ollama pull`. Notably gives Apple-Silicon adopters working GPU acceleration out of the box (via llama.cpp Metal), which today only the fragile sidecar delivers.
- **Perf:** Easiest to stand up; one HTTP POST, zero CGO/build/model-download friction beyond the pull.
- **Retrieval quality:** **Dense-only, 1024-dim BGE-M3 dense.** Ollama exposes only the dense head — it **cannot** surface BGE-M3's sparse or ColBERT modalities (llama.cpp runs only the dense pooling path). This means it implements `Embedder` but **not** `FullEmbedder`, silently degrading the 4-way hybrid to **2-way (fts + dense)**. The dense vectors *are* index-compatible with the existing ORT BGE-M3 dense lane (identical weights, CLS pooling, L2-normalization, 1024 dims — F16 vs fp32 drift is negligible for cosine/RRF), so no dense reindex is forced **provided `/api/embed` is used**. Still strictly **better than MiniLM** (1024-dim BGE-M3 dense vs 384-dim MiniLM), just below the full hybrid.
- **How to select it:** N/A — requires building an `OllamaEmbedder`. If built, it must be stamped as a **distinct dense-only model identity** so the mixed-model guardrail (which keys "is this a bge-m3 vault" on populated `sparse_embedding`/`colbert_embedding` columns) is not fooled by an Ollama vault that stamps `model="bge-m3"` but leaves those columns NULL.
- **Status:** Evaluated, not implemented.

## Adoption matrix

| Backend | System deps | Platforms | Indexing speed (BGE-M3, ~130 notes) | Retrieval quality | Adoption ease |
|---|---|---|---|---|---|
| **Prebuilt ORT archive** *(from v0.1.1)* | None — libonnxruntime bundled | darwin-arm64, linux-amd64 | ~2–3 min (est.) | **Full 4-way hybrid** | **Easiest full-hybrid**: download `*_ort.tar.gz`, extract, run — no build |
| **Pure-Go MiniLM** | None (static binary) | All incl. Windows | N/A — MiniLM only; BGE-M3 here is *hours*, hard-blocked | **Dense-only, 384-dim** | `go install …@latest`, or `task build:fast` |
| **ORT-CPU (from source)** | libonnxruntime + libtokenizers.a + CGO | macOS arm64/x86_64, Linux x86_64/arm64 (no Windows) | ~2–3 min (est.) | **Full 4-way hybrid** | Medium: brew + setup:ort + CGO + 2.2GB model pull (for platforms without a prebuilt archive, or contributors) |
| **ORT+CoreML** | ORT deps + macOS CoreML frameworks | macOS arm64 only | — (blocked) | Would be 4-way (none today) | **Blocked** (issue #34) |
| **MPS sidecar** | ORT deps + Python + torch + transformers (+MPS) | Apple Silicon GPU; CPU elsewhere | ~4× ORT-CPU (+~4s start) | **Full 4-way hybrid** | Hardest working path: venv + 3 env vars + abs script path |
| **Ollama** *(unimplemented)* | Ollama daemon only | All incl. Windows | Fast (GPU via Metal/llama.cpp) | **Dense-only, 1024-dim** | **Easiest to stand up** (one `ollama pull`); needs new `OllamaEmbedder` |

## CoreML status (issue #34)

The in-process Apple-Silicon GPU path (CoreML execution provider) is **blocked** and has been since the 2026-04-29 investigation. Three independent upstream blockers prevent CoreML from running BGE-M3 in the pinned stack (hugot v0.7.0 / onnxruntime_go v1.30.1 / libonnxruntime 1.25.0):

1. **External-data file-size resolver error** — session creation fails with `ReadExternalDataForTensor Failed to get file size ... std::filesystem error: Not a directory`. (BGE-M3 ships weights as external data in `model.onnx_data`, ~2.1GB.)
2. **Absolute-path security guard** on external-data resolution.
3. **2GB protobuf limit** — BGE-M3 weights (~2.2GB fp32) exceed the protobuf message ceiling.

**Upstream re-check (as of this writing):** hugot has released v0.7.1–v0.7.4 with no CoreML notes; onnxruntime_go is already at the latest v1.30.1; ONNX Runtime v1.26.0's CoreML changes are operators-only. The CoreML EP docs still omit external-data and the 2GB case, and no upstream issue or PR fixing the resolver error was found. **Verdict: likely not fixed.** Keep issue #34 parked; keep the MPS sidecar as the only working Apple-Silicon GPU path.

**How to cheaply re-test when an upstream fix is suspected:** bump hugot to its latest release, rebuild with `-tags ORT` on darwin/arm64, and run an index with `VAULTMIND_ENABLE_COREML=1`. Success = a clean session creation and `Acceleration()` reporting `ort+coreml`; failure = the external-data resolver error above.

**Live re-test result:**

**Re-tested 2026-06-03** on the *same* stack issue #34 first documented (hugot v0.7.0, `yalue/onnxruntime_go` v1.30.1, libonnxruntime 1.25.0), BGE-M3 fp32 cached locally. Running `VAULTMIND_ENABLE_COREML=1 vaultmind index --embed --model bge-m3` reproduces **blocker #1 verbatim**:

```
creating BGE-M3 pipeline: Error creating C session from file:
tensorprotoutils.cc:234 ReadExternalDataForTensor Failed to get file size for
external initializer 0.auto_model.embeddings.position_embeddings.weight.
std::filesystem error: Not a directory (value: 20)
```

The runtime **gracefully fell back to MiniLM** (warned, did not crash). CoreML remains blocked at the current pins. Re-test cadence: whenever hugot or onnxruntime is bumped (see issue #34 for the upstream conditions that would unblock it).

## Ollama evaluation

Ollama is the **easiest backend to stand up** and sidesteps every hard part of VaultMind's current stack: no CGO, no `libonnxruntime`/`libtokenizers`, no `-tags ORT` build, no ~2.2GB ONNX download wrangling, no Python+PyTorch+MPS sidecar. It works uniformly across macOS arm64/x86_64, Linux, and Windows with one `ollama pull`, and it gives Apple-Silicon users working GPU acceleration out of the box (llama.cpp Metal) — something only the fragile sidecar delivers today.

**The non-negotiable tradeoff: Ollama is dense-only.** Its embedding API exposes only a single dense vector per input; it cannot surface BGE-M3's sparse (lexical-exact-match) or ColBERT (token-level late-interaction) heads, even though the underlying model supports all three. Consequence: an Ollama-embedded vault never populates the `sparse_embedding`/`colbert_embedding` columns, so VaultMind's signature **4-way RRF hybrid silently degrades to 2-way (fts + dense)**. No error is raised — the index builds fine but the retrieval-quality differentiators that justified choosing BGE-M3 over MiniLM are simply gone. **This is a retrieval-quality regression masquerading as a working index, not a UX tweak.**

The dense lane itself *is* genuinely compatible: identical BGE-M3 weights, identical CLS pooling, identical L2-normalization, identical 1024 dims (F16-vs-fp32 drift is negligible for cosine/RRF). So Ollama dense vectors interoperate with existing ORT BGE-M3 dense rows **only if `/api/embed` is used** — the legacy `/api/embeddings` endpoint returns *unnormalized* vectors that would silently break cosine parity.

| Ollama is right for… | Ollama is wrong for… |
|---|---|
| New adopters wanting one-command setup | Anyone who needs VaultMind's best retrieval |
| Evaluation / low-stakes tier | A vault meant to use the 4-way hybrid |
| Users who'd otherwise get pure-Go MiniLM (Ollama's 1024-dim dense is strictly better) | Mixing Ollama rows into a full ORT BGE-M3 vault (inconsistent-hybrid state the guardrail wasn't designed to catch) |
| Windows users (no ORT fast path exists for them) | Production retrieval where sparse/ColBERT signal matters |

**If implemented, the guardrails are mandatory:** wire an `OllamaEmbedder` as an `Embedder` (not a `FullEmbedder`), pin it to `/api/embed`, stamp it as a **distinct dense-only model identity** (so the mixed-model guardrail stays honest), and have `index --embed` print a loud warning that sparse + ColBERT lanes are disabled. Never make it the silent default for a hybrid vault.

**Live vector-compatibility probe:**

**Probed 2026-06-03** with `ollama pull bge-m3` (1.2 GB **quantized**, vs the 2.2 GB fp32 ORT model). For an identical sentence, Ollama's `/api/embed` and vaultmind's ORT fp32 dense output were measured directly:

| | dims | L2 norm | cosine(Ollama, ORT) |
|---|---|---|---|
| ORT fp32 (in-process) | 1024 | 1.000000 | — |
| Ollama (quantized daemon) | 1024 | 1.000000 | **0.99999** |

**The dense path is interchangeable** — Ollama's quantized vectors are functionally identical to ORT's fp32 dense vectors and index-compatible (same dims, pre-normalized). **The catch:** Ollama's embedding API returns **dense only**. It cannot emit BGE-M3's *sparse* (lexical) or *ColBERT* (multi-vector) outputs, so an Ollama-backed vault runs **dense-only retrieval**, not the 4-way RRF hybrid. You keep ~MiniLM-grade-or-better dense semantics with BGE-M3's 1024-dim space and 8k context, but you lose the lexical + late-interaction signals that make the hybrid path stronger on exact-term and long-document recall.

## Recommendation

**The single recommended default adoption path: the full BGE-M3 4-way hybrid**, because retrieval quality is fixed at index time and a vault silently indexed dense-only can never be "upgraded" by the query side — only by a full re-index. Per platform:

- **macOS arm64 / Linux amd64:** the **prebuilt ORT archive** (download `vaultmind_<ver>_<os>_<arch>_ort.tar.gz`, extract, run — libonnxruntime bundled, no build). This is the easiest full-hybrid path. Build from source (`brew install onnxruntime && task setup:ort && task build`) only if you're a contributor or need a different revision. Reserve the MPS sidecar as an opt-in "fast index" upgrade for large/frequently-changing vaults only.
- **macOS x86_64 / Linux arm64:** no prebuilt archive yet → ORT-CPU from source (`task setup:ort && task build`; `brew install onnxruntime` or package/release for libonnxruntime). Full hybrid with no GPU/Python complexity.
- **Windows:** Pure-Go MiniLM, documented explicitly as a known limitation — there is no ORT fast path (bash `setup-ort.sh` has no Windows branch; CGO + native libs are untested).
- **Any platform, low-stakes / one-command setup:** Ollama dense-only (once implemented) or pure-Go MiniLM — always as an explicit, clearly-labeled fallback, never the silent default.

**Why the defaults must diverge (AX vs UX):** the AI consumer hits the *query* path and wants the richest possible index (full BGE-M3); it's indifferent to how painful the one-time install was, and it can't detect a dense-only index from the query side. The human operator hits the *install* path and wants minimal friction — and their cheapest path (Ollama dense-only, or skipping `setup:ort`) is exactly the one that degrades the consumer's index forever. So optimize the operator's install for the *least friction that still yields a full BGE-M3 index*, and treat dense-only shortcuts as labeled fallbacks the operator opts into knowingly.

**Concrete product changes that would most reduce friction, in priority order:**

1. **Prebuilt ORT release binaries** ✅ *shipping from v0.1.1* (highest leverage). A CI matrix (`ort-release` job) builds `-tags ORT` per platform (darwin-arm64, linux-amd64) and bundles the official self-contained `libonnxruntime` beside the binary; `detectORTLibDir` checks the executable's own directory so it's found with zero config. `libtokenizers` is statically linked. Adopters download `vaultmind_<ver>_<os>_<arch>_ort.tar.gz`, extract, and run — the full hybrid with no `brew`, no `setup:ort`, no source build. This collapses "clone + task build + brew + setup:ort" into "download one archive."
2. **Add an Ollama easy-mode backend, scoped honestly.** Wire it as an `Embedder` (not `FullEmbedder`), pin to `/api/embed`, stamp as a distinct dense-only identity, and print at index time: *"⚠ Ollama backend is dense-only — BGE-M3 sparse+ColBERT lanes are disabled; retrieval quality is reduced. For full hybrid use the ORT build."* The frictionless on-ramp for evaluation, never the silent default for a hybrid vault.
3. **Auto-detect + doctor-led backend selection.** Add `vaultmind doctor --backends` (or surface `Acceleration()`, which is computed today but has **zero callers** outside the embedding package) to probe the machine once and print a ranked, copy-pasteable plan: `libonnxruntime: found ✓ | libtokenizers.a: missing → run task setup:ort | Ollama: reachable, bge-m3 pulled (dense-only) | torch/MPS sidecar: torch not installed`. The tool already knows its acceleration state and currently hides it.
4. **Make the silent downgrade loud at index time, not just in doctor.** When `index --embed` resolves to MiniLM via `DefaultModel()` on a pure-Go build, the quiet `[model: minilm]` substring is too easy to miss. Add: *"Indexed with MiniLM dense-only (384d). For BGE-M3 4-way hybrid: task setup:ort && task build, then re-embed."* The consumer's index quality is being decided here — surface it where the decision is made.
5. **Pre-flight the ~2.2GB model download.** Before the first BGE-M3 embed, check free disk + network reachability and print expected size/time; add resume-on-partial so a dropped connection doesn't restart a 2.1GB file from zero.
