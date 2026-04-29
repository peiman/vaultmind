#!/usr/bin/env python3
"""BGE-M3 embedding sidecar — loads the model into memory once, then serves
embedding requests over stdin/stdout via newline-delimited JSON.

Why this exists: in-process ORT on Go (via hugot) saturates CPU during
indexing — fans go through the roof on Apple Silicon because there's no
GPU acceleration path. This sidecar uses PyTorch with the MPS (Metal
Performance Shaders) backend, which routes transformer inference through
Apple Silicon's GPU. Same model, different engine.

Vaultmind's Go side launches this as a subprocess for the duration of an
indexing pass and tears it down after. Tokenization stays in Go; only the
heavy transformer inference + the sparse/ColBERT heads run here. The
process boundary is the architectural fix per arc-closing-at-the-right-
layer: vaultmind core stays untouched; the inference engine is swappable
behind a JSON contract.

Protocol (NDJSON over stdin/stdout):
  Request:  {"texts": ["<note body>", ...]}
  Response: {"dense": [[float...], ...],
             "sparse": [{"<token_id>": <weight>, ...}, ...],
             "colbert": [[[float...], ...], ...]}
  Errors:   {"error": "<message>"}
  Ready:    {"ready": true, "device": "mps"|"cpu"}  (sent on startup)

Lifecycle: ready signal on startup, then loops reading lines until EOF or
the parent closes stdin.
"""

import json
import os
import sys
import warnings
from pathlib import Path

warnings.filterwarnings("ignore")


def emit(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


def main():
    try:
        import torch
        from transformers import AutoModel, AutoTokenizer
    except Exception as e:
        emit({"error": f"import failed: {type(e).__name__}: {e}"})
        sys.exit(2)

    device = "mps" if torch.backends.mps.is_available() else "cpu"
    dtype = torch.float16 if device == "mps" else torch.float32

    cache_dir = os.path.expanduser("~/.vaultmind/models")
    model_name = "BAAI/bge-m3"

    try:
        tokenizer = AutoTokenizer.from_pretrained(model_name, cache_dir=cache_dir)
        model = AutoModel.from_pretrained(model_name, cache_dir=cache_dir, torch_dtype=dtype)
        model = model.to(device)
        model.eval()
    except Exception as e:
        emit({"error": f"model load failed: {type(e).__name__}: {e}"})
        sys.exit(2)

    bgem3_dir = Path(cache_dir) / "BAAI_bge-m3"
    sparse_w_path = bgem3_dir / "sparse_linear.pt"
    colbert_w_path = bgem3_dir / "colbert_linear.pt"

    if not sparse_w_path.exists() or not colbert_w_path.exists():
        emit({"error": f"head weights missing at {bgem3_dir} — run vaultmind index --embed --model bge-m3 once first to populate the cache"})
        sys.exit(2)

    sparse_state = torch.load(sparse_w_path, map_location=device, weights_only=True)
    colbert_state = torch.load(colbert_w_path, map_location=device, weights_only=True)

    sparse_w = sparse_state["weight"].to(device, dtype=dtype)
    sparse_b = sparse_state["bias"].to(device, dtype=dtype)
    colbert_w = colbert_state["weight"].to(device, dtype=dtype)
    colbert_b = colbert_state["bias"].to(device, dtype=dtype)

    emit({"ready": True, "device": device})

    max_len = 8190

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as e:
            emit({"error": f"bad request: {e}"})
            continue

        texts = req.get("texts")
        if not isinstance(texts, list) or not texts:
            emit({"error": "request must have non-empty texts array"})
            continue

        try:
            # Process each text in its own forward pass.
            #
            # Why one-at-a-time: when multiple variable-length texts are
            # batched into a single forward pass, BGE-M3's attention
            # masking diverges between this PyTorch+MPS path and
            # vaultmind's reference in-process ORT path, producing
            # dense_cos drift of 0.15-0.36 for shorter-than-max texts
            # within the batch. Single-text forwards produce dense_cos
            # = 1.0000 against the reference. Speed cost: ~1.04x slower
            # vs batched (measured via TestSidecar_Speed_SingleVsBatched).
            # Correctness restoration is essentially free.
            #
            # The protocol stays batched at the JSON layer so callers do
            # not change. Future: if upstream attention-mask handling is
            # fixed for batched inputs, we can swap back without
            # touching the Go side.
            dense_list = []
            sparse_list = []
            colbert_list = []
            with torch.no_grad():
                for text in texts:
                    enc = tokenizer(
                        [text],
                        padding=True,
                        truncation=True,
                        max_length=max_len,
                        return_tensors="pt",
                    ).to(device)

                    outputs = model(**enc, return_dict=True)
                    hidden = outputs.last_hidden_state  # [1, L, 1024]

                    dense = torch.nn.functional.normalize(hidden[:, 0, :], p=2, dim=1)

                    sparse_scores = torch.relu(hidden @ sparse_w.t() + sparse_b).squeeze(-1)
                    input_ids = enc["input_ids"]
                    special_mask = enc.get("special_tokens_mask")
                    if special_mask is None:
                        special_ids = set(tokenizer.all_special_ids)
                        special_mask = torch.zeros_like(input_ids)
                        for sid in special_ids:
                            special_mask = special_mask | (input_ids == sid).int()

                    colbert = torch.nn.functional.normalize(
                        hidden @ colbert_w.t() + colbert_b, p=2, dim=2,
                    )

                    dense_list.append(dense[0].cpu().float().tolist())

                    sparse_dict = {}
                    ss = sparse_scores[0].cpu().float()
                    ids = input_ids[0].cpu()
                    mask = special_mask[0].cpu()
                    attn = enc["attention_mask"][0].cpu()
                    for i in range(len(ss)):
                        if mask[i] == 1 or attn[i] == 0:
                            continue
                        val = float(ss[i])
                        if val <= 0:
                            continue
                        tid = int(ids[i])
                        if val > sparse_dict.get(tid, 0.0):
                            sparse_dict[tid] = val
                    sparse_list.append({str(k): v for k, v in sparse_dict.items()})

                    cb = colbert[0].cpu().float()
                    kept = []
                    for i in range(1, cb.shape[0]):
                        if mask[i] == 1 or attn[i] == 0:
                            continue
                        kept.append(cb[i].tolist())
                    colbert_list.append(kept)

            emit({"dense": dense_list, "sparse": sparse_list, "colbert": colbert_list})
        except Exception as e:
            import traceback
            emit({"error": f"inference failed: {type(e).__name__}: {e}", "trace": traceback.format_exc()})


if __name__ == "__main__":
    main()
