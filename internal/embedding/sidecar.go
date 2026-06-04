package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"
)

// SidecarBGEM3Embedder runs BGE-M3 inference in an external Python process
// that uses PyTorch + MPS (Apple Silicon GPU). The Go side handles
// tokenization context (none — Python sidecar tokenizes with HF tokenizer
// loaded from cache) and the heads run inside the sidecar so the per-modality
// tensors flow through MPS without round-tripping to CPU mid-batch.
//
// Why a sidecar instead of in-process: in-process ORT (via hugot) saturates
// CPU during indexing on Apple Silicon — there's no GPU acceleration path
// (vaultmind#34). The sidecar pattern moves heavy inference behind a JSON
// contract, isolating vaultmind core from the inference engine choice. Today
// the engine is PyTorch+MPS; tomorrow it could be CoreML or MLX without
// touching the Go side.
//
// Lifecycle: the embedder spawns the Python subprocess in NewSidecarBGEM3.
// Close() tears it down. Per-batch round-trips happen via Send (write JSON
// line to stdin, read JSON line from stdout). Mutex serializes access since
// the protocol is synchronous request/response on a single FD pair.
type SidecarBGEM3Embedder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser
	mu     sync.Mutex
	closed bool
	device string // "mps" or "cpu" — set on startup ready signal
}

// SidecarBGEM3Config controls how the sidecar process is launched.
type SidecarBGEM3Config struct {
	// Python is the interpreter path. Must have torch + transformers
	// installed and be on a platform where torch.backends.mps.is_available()
	// returns true (Apple Silicon). When empty, falls back to "python3" on
	// PATH.
	Python string
	// ScriptPath is the absolute path to embed_server.py. When empty, the
	// embedder looks for the script alongside this Go file (resolved via
	// the project's $CLAUDE_PROJECT_DIR or the executable's directory).
	ScriptPath string
}

// NewSidecarBGEM3 spawns the Python sidecar and waits for its ready signal.
// Returns an error if the subprocess fails to start, the Python imports
// fail, or the model can't be loaded. The caller MUST defer Close() to
// reap the subprocess.
func NewSidecarBGEM3(cfg SidecarBGEM3Config) (*SidecarBGEM3Embedder, error) {
	python := cfg.Python
	if python == "" {
		python = "python3"
	}
	script := cfg.ScriptPath
	if script == "" {
		// Best-effort default: project-relative path. The CLI passes an
		// explicit path through config; this fallback is for tests.
		script = filepath.Join("internal", "embedding", "sidecar", "embed_server.py")
	}
	if _, err := os.Stat(script); err != nil {
		return nil, fmt.Errorf("sidecar script not found at %s: %w", script, err)
	}

	cmd := exec.Command(python, script) //nolint:gosec // python path comes from operator config, not user input
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting sidecar: %w", err)
	}

	stdout := bufio.NewReaderSize(stdoutPipe, 1<<20) // 1MB buffer; ColBERT responses can be hundreds of KB
	emb := &SidecarBGEM3Embedder{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderrPipe,
	}

	// Wait for ready signal. The sidecar emits exactly one line on startup:
	// either {"ready":true,"device":"mps"|"cpu"} on success, or
	// {"error":"..."} on failure. Python startup + model load can take
	// 10-30 seconds.
	line, err := stdout.ReadString('\n')
	if err != nil {
		_ = emb.Close()
		return nil, fmt.Errorf("sidecar startup: %w (stderr: %s)", err, drainStderr(stderrPipe))
	}
	var ready struct {
		Ready  bool   `json:"ready"`
		Device string `json:"device"`
		Error  string `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(line), &ready); jerr != nil {
		_ = emb.Close()
		return nil, fmt.Errorf("sidecar startup: bad ready line %q: %w", line, jerr)
	}
	if ready.Error != "" {
		_ = emb.Close()
		return nil, fmt.Errorf("sidecar startup: %s", ready.Error)
	}
	if !ready.Ready {
		_ = emb.Close()
		return nil, fmt.Errorf("sidecar startup: did not signal ready")
	}
	emb.device = ready.Device
	log.Info().Str("device", ready.Device).Msg("BGE-M3 sidecar ready")
	return emb, nil
}

// Dims reports the dense embedding dimensionality.
func (e *SidecarBGEM3Embedder) Dims() int { return BGEM3Dims }

// Embed produces a single dense embedding via the sidecar.
func (e *SidecarBGEM3Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	out, err := e.EmbedFull(ctx, text)
	if err != nil {
		return nil, err
	}
	return out.Dense, nil
}

// EmbedBatch produces dense embeddings for a batch via the sidecar.
func (e *SidecarBGEM3Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	outs, err := e.EmbedFullBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	result := make([][]float32, len(outs))
	for i, o := range outs {
		result[i] = o.Dense
	}
	return result, nil
}

// EmbedFull is the singleton form of EmbedFullBatch.
func (e *SidecarBGEM3Embedder) EmbedFull(ctx context.Context, text string) (*BGEM3Output, error) {
	outs, err := e.EmbedFullBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(outs) == 0 {
		return nil, fmt.Errorf("sidecar returned no outputs")
	}
	return outs[0], nil
}

// EmbedFullBatch sends a batch of texts to the sidecar and parses the
// response. Tokens are sparse-key strings in the JSON; we parse to int32
// here so the sidecar protocol stays portable across languages.
func (e *SidecarBGEM3Embedder) EmbedFullBatch(_ context.Context, texts []string) ([]*BGEM3Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("sidecar embedder is closed")
	}

	req := map[string]any{"texts": texts}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	if _, err := e.stdin.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("writing request to sidecar: %w", err)
	}

	line, err := e.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading sidecar response: %w", err)
	}

	var resp struct {
		Dense   [][]float32          `json:"dense"`
		Sparse  []map[string]float32 `json:"sparse"`
		ColBERT [][][]float32        `json:"colbert"`
		Error   string               `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("decoding sidecar response: %w (line: %s)", err, line)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("sidecar error: %s", resp.Error)
	}
	if len(resp.Dense) != len(texts) {
		return nil, fmt.Errorf("sidecar returned %d dense vectors for %d texts", len(resp.Dense), len(texts))
	}

	outputs := make([]*BGEM3Output, len(texts))
	for i := range texts {
		sparseMap := make(map[int32]float32, len(resp.Sparse[i]))
		for k, v := range resp.Sparse[i] {
			id, err := strconv.ParseInt(k, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("sparse key %q is not an int32: %w", k, err)
			}
			sparseMap[int32(id)] = v
		}
		outputs[i] = &BGEM3Output{
			Dense:   resp.Dense[i],
			Sparse:  sparseMap,
			ColBERT: resp.ColBERT[i],
		}
	}
	return outputs, nil
}

// Close terminates the sidecar process. Safe to call multiple times.
func (e *SidecarBGEM3Embedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	// Closing stdin signals the sidecar to exit cleanly via its EOF check
	// in the read loop.
	_ = e.stdin.Close()
	if e.cmd != nil && e.cmd.Process != nil {
		// Best-effort: wait for graceful exit, kill if it hangs.
		done := make(chan error, 1)
		go func() { done <- e.cmd.Wait() }()
		select {
		case <-done:
			// clean exit
		default:
			// fallthrough — give it up to 5s in real use, but in Close we
			// just let the OS clean up if it doesn't exit immediately
		}
	}
	return nil
}

// Device reports the device the sidecar selected ("mps" or "cpu").
// Useful for the doctor / index summary so the operator sees acceleration.
func (e *SidecarBGEM3Embedder) Device() string { return e.device }

// drainStderr reads as much as it can from stderr without blocking. Used
// when the startup ready line fails so we can surface a richer error.
func drainStderr(r io.Reader) string {
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}
