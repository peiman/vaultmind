# BGE-M3 Embedding Sidecar

A Python subprocess that runs BGE-M3 inference through PyTorch on Apple
Silicon's GPU (MPS) instead of saturating CPU cores via the in-process
ORT path. Vaultmind's Go side spawns it during indexing, sends batches
of note bodies as NDJSON over stdin, and reads back dense + sparse +
ColBERT outputs.

## Why this exists

The in-process path runs ONNX Runtime through CGO (via hugot). On Apple
Silicon, ORT's CoreML execution provider can't load BGE-M3 due to three
upstream blockers (see vaultmind#34). The sidecar pattern moves heavy
inference behind a JSON contract — Apple Silicon GPU acceleration via
PyTorch+MPS, with vaultmind core staying untouched. The architectural
boundary is the fix per `arc-closing-at-the-right-layer`: don't rewrite
the inference engine in Go, swap engines through a process boundary.

## Measured impact (on 34-note identity vault clone)

| Path | Wall time | CPU | Notes |
|---|---|---|---|
| In-process ORT+CPU | 3:42 | 700%+ sustained | Fans saturated |
| Sidecar PyTorch+MPS | ~1:15 | ~17% | ~5s startup, ~0.6s/note inference |

~3x faster wall time, ~40x less CPU saturation. Heat reduction is the
load-bearing win — fans barely engage because most work is on GPU.

## Numerical equivalence (correctness gate)

The sidecar produces embeddings **numerically equivalent** to the in-process
ORT+CPU reference. Verified by `TestSidecar_NumericalEquivalence_VsInProcess`:

- Dense cosine ≥ 0.9999 across all tested texts
- ColBERT per-token cosine ≥ 0.9999
- Sparse top-10 token overlap ≥ 0.7 (small differences are tokenization
  variance, not engine drift)

This means switching between sidecar and in-process is safe: a vault
embedded via the sidecar produces the same retrieval rankings as a vault
embedded in-process. Mixed-state vaults (some notes via sidecar, others
in-process) work correctly.

**Why each text gets its own forward pass:** when multiple variable-length
texts are batched together for a single forward pass, BGE-M3's attention
masking diverges between PyTorch+MPS and ORT+CPU on shorter-than-max texts
in the batch — empirically dense cosine drops to 0.15-0.36 for affected
texts. The sidecar loops one text at a time internally; the JSON protocol
stays batched so the Go side is unchanged. Speed cost of singleton vs
batched mode: ~1.04x (measured) — essentially free, because the work IS
the transformer forward pass and that fully utilizes MPS regardless.
Correctness restored at no perceptible cost.

This was caught by probe-before-commit: the 4.28x "speedup" measured on
the original batched-mode sidecar was on incorrect outputs. Running the
numerical equivalence probe BEFORE shipping caught the regression. Lesson
captured in `reference-probe-before-commit` (identity vault).

## Setup (one-time)

Requires Python 3.11/3.12/3.13 with PyTorch + transformers. A clean venv
is the simplest path:

```bash
# Create venv (replace 3.13 with whatever Python version you have)
python3 -m venv ~/.vaultmind/sidecar-venv

# Install dependencies
~/.vaultmind/sidecar-venv/bin/pip install torch transformers
```

First run will download BGE-M3 (~2GB) into the HuggingFace cache. After
that, model loads from disk in ~4 seconds.

## Activation

Set environment variables when running indexing:

```bash
export VAULTMIND_USE_SIDECAR=1
export VAULTMIND_SIDECAR_PYTHON=$HOME/.vaultmind/sidecar-venv/bin/python
export VAULTMIND_SIDECAR_SCRIPT=/path/to/vaultmind/internal/embedding/sidecar/embed_server.py

vaultmind index --vault <vault> --embed --model bge-m3
```

Look for `BGE-M3 sidecar active for indexing device=mps` in the log.

If the sidecar fails to start (Python missing, deps missing, model load
fails), vaultmind falls back to in-process ORT+CPU automatically. The
embed pass still completes — graceful degradation.

## Falls back to in-process when

- `VAULTMIND_USE_SIDECAR` is not set (default behavior — opt-in only)
- `VAULTMIND_SIDECAR_PYTHON` doesn't point to a Python with torch+transformers
- `VAULTMIND_SIDECAR_SCRIPT` is missing or unreadable
- Sidecar startup signals an error (model load failure, head weights missing)

In all these cases the user sees a single `WARN` log line naming the
failure and the in-process path continues. No data loss, just no
acceleration.

## Protocol (NDJSON over stdin/stdout)

Startup signal:

```json
{"ready": true, "device": "mps"}
```

Or on failure:

```json
{"error": "model load failed: ..."}
```

Per-batch request (Go → Python):

```json
{"texts": ["<note body 1>", "<note body 2>", ...]}
```

Per-batch response (Python → Go):

```json
{
  "dense":   [[float...], ...],
  "sparse":  [{"<token_id>": <weight>, ...}, ...],
  "colbert": [[[float...], ...], ...]
}
```

Or:

```json
{"error": "<message>"}
```

Lifecycle: subprocess starts on demand inside `RunEmbed`, runs for the
duration of the indexing pass (handles all batches in a single process),
exits when stdin closes.

## When the sidecar earns its keep

- Full vault re-embeds (most of the heat)
- Vaults > 50 notes pending re-embed in a single run

When NOT to use the sidecar:

- Short query embedding (`vaultmind ask`): the 4-second startup
  dominates a single-query workload. Ask uses in-process ORT regardless.
- Tiny incremental indexes (1-2 notes pending): startup overhead exceeds
  the work. The lazy-load fix handles 0-pending cases; for 1-2 pending,
  in-process is faster end-to-end. Could add a threshold flag later
  if useful.

## Tests

`internal/embedding/sidecar_bench_test.go` — dev-tagged, env-gated. Run:

```bash
VAULTMIND_SIDECAR_BENCH=1 \
VAULTMIND_SIDECAR_PYTHON=$HOME/.vaultmind/sidecar-venv/bin/python \
CGO_LDFLAGS="-L$(pwd)/lib" \
go test -tags "dev ORT" -count=1 -v -run TestSidecar_VsInProcess_Throughput \
  ./internal/embedding/
```

Reports in-process vs sidecar timing per-note plus overall speedup.

## Cross-references

- vaultmind#34 — the CoreML EP gap that motivated this sidecar
- `arc-closing-at-the-right-layer` (identity vault) — the principle
  behind choosing the process-boundary architectural layer
- `reference-complexity-not-time` — the meta-correction that reframed
  this work from "Large" to "Medium"
