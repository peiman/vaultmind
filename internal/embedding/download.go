package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// downloadClient is an HTTP client with a generous timeout for large model downloads.
var downloadClient = &http.Client{Timeout: 30 * time.Minute}

// huggingFaceBase is where pinned model files are fetched from. A variable
// so a test can point it at a local server.
var huggingFaceBase = "https://huggingface.co"

// modelFile is one file of a pinned model and what it must be.
type modelFile struct {
	remote string
	local  string
	size   int64
	sha256 string
}

// pinnedModel is a model at a fixed commit: `main` moves, and a pinned commit
// with a hash per file means the bytes we run are the bytes we checked (#192,
// #197, review M7).
type pinnedModel struct {
	repo     string
	revision string
	files    []modelFile
}

// pinnedModels are the only models VaultMind downloads. Sizes and hashes are
// HuggingFace's LFS records for each commit, or the sha256 of the served bytes
// for files LFS does not track; every file was also checked against a cache in
// use. A model not in this table is refused rather than fetched unchecked.
var pinnedModels = map[string]pinnedModel{
	BGEM3ModelName: {repo: BGEM3ModelName, revision: "5617a9f61b028005a4858fdac845db406aefb181", files: bgem3Files},
	DefaultModelName: {repo: DefaultModelName, revision: "1110a243fdf4706b3f48f1d95db1a4f5529b4d41", files: []modelFile{
		{"onnx/model.onnx", "model.onnx", 90405214, "6fd5d72fe4589f189f8ebc006442dbb529bb7ce38f8082112682524616046452"},
		{"config.json", "config.json", 612, "953f9c0d463486b10a6871cc2fd59f223b2c70184f49815e7efbcab5d8908b41"},
		{"tokenizer.json", "tokenizer.json", 466247, "be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037"},
		{"tokenizer_config.json", "tokenizer_config.json", 350, "acb92769e8195aabd29b7b2137a9e6d6e25c476a4f15aa4355c233426c61576b"},
		{"special_tokens_map.json", "special_tokens_map.json", 112, "303df45a03609e4ead04bc3dc1536d0ab19b5358db685b6f3da123d05ec200e3"},
		{"vocab.txt", "vocab.txt", 231508, "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3"},
	}},
}

// bgem3Files are the BGE-M3 files at its pinned commit.
var bgem3Files = []modelFile{
	{"onnx/model.onnx", "model.onnx", 724923, "f84251230831afb359ab26d9fd37d5936d4d9bb5d1d5410e66442f630f24435b"},
	{"onnx/model.onnx_data", "model.onnx_data", 2266820608, "1eebfb28493f67bba03ce0ef64bfdc7fc5a3bd9d7493f818bb1d78cd798416b4"},
	{"tokenizer.json", "tokenizer.json", 17098108, "21106b6d7dab2952c1d496fb21d5dc9db75c28ed361a05f5020bbba27810dd08"},
	{"tokenizer_config.json", "tokenizer_config.json", 444, "a62b2b6784f990259fddef5f16388693a8043be4f69179e6a5257eeb3f9abac4"},
	{"special_tokens_map.json", "special_tokens_map.json", 964, "8c785abebea9ae3257b61681b4e6fd8365ceafde980c21970d001e834cf10835"},
	{"config.json", "config.json", 687, "26159e7ad065073448460117eb24b7a4572f6f4e78eadff65dc0a11c052449fa"},
	{"sparse_linear.pt", "sparse_linear.pt", 3516, "45c93804d2142b8f6d7ec6914ae23a1eee9c6a1d27d83d908a20d2afb3595ad9"},
	{"colbert_linear.pt", "colbert_linear.pt", 2100674, "19bfbae397c2b7524158c919d0e9b19393c5639d098f0a66932c91ed8f5f9abb"},
}

// DownloadBGEM3 makes sure the pinned BGE-M3 files are in the cache and
// match their hashes, downloading what is missing or wrong. Returns the path
// to the model directory.
func DownloadBGEM3(cacheDir string) (string, error) {
	return downloadPinned(cacheDir, BGEM3ModelName)
}

// downloadPinned makes sure the pinned files of the named model are in the
// cache and match their hashes. Returns the model directory — the layout the
// embedding pipelines read, <cache>/<owner>_<name>.
func downloadPinned(cacheDir, name string) (string, error) {
	m, ok := pinnedModels[name]
	if !ok {
		return "", fmt.Errorf("model %q has no pinned files; only pinned models are downloaded", name)
	}
	modelDir := hugotModelDir(cacheDir, name)
	base := fmt.Sprintf("%s/%s/resolve/%s", huggingFaceBase, m.repo, m.revision)
	if err := ensureModel(base, modelDir, m.revision, m.files); err != nil {
		return "", err
	}
	return modelDir, nil
}

// ensureModel makes modelDir hold exactly files, as of revision.
//
// Every download is hashed as it streams and refused unless size and sha256
// match. A cache that predates verification is hashed once; a marker named
// for the revision then records it, so a normal load only compares sizes —
// hashing 2.2 GB on every `ask` would cost seconds each time. A file whose
// size changes is hashed again.
func ensureModel(baseURL, modelDir, revision string, files []modelFile) error {
	if err := os.MkdirAll(modelDir, 0o750); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}
	marker := filepath.Join(modelDir, ".verified-"+revision)
	_, markerErr := os.Stat(marker)
	verified := markerErr == nil
	announced := false
	for _, f := range files {
		path := filepath.Join(modelDir, f.local)
		if ok, err := cached(path, f, verified); err != nil {
			return err
		} else if ok {
			continue
		}
		if !announced {
			fmt.Fprintf(os.Stderr, "Model files missing or not verified. Downloading to %s\n", modelDir)
			announced = true
		}
		if err := downloadVerified(baseURL+"/"+f.remote, path, f); err != nil {
			return fmt.Errorf("downloading %s: %w", f.local, err)
		}
	}
	if verified {
		return nil
	}
	return os.WriteFile(marker, []byte(revision+"\n"), 0o600)
}

// cached reports whether path already holds f: the right size, and — unless
// the cache was verified for this revision — the right hash.
func cached(path string, f modelFile, verified bool) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() != f.size {
		fmt.Fprintf(os.Stderr, "  %s: %d bytes, want %d — downloading again\n", f.local, info.Size(), f.size)
		return false, nil
	}
	if verified {
		return true, nil
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return false, err
	}
	if sum != f.sha256 {
		fmt.Fprintf(os.Stderr, "  %s: does not match its pinned sha256 — downloading again\n", f.local)
		return false, nil
	}
	return true, nil
}

func fileSHA256(path string) (string, error) {
	// nosemgrep: go-path-traversal -- the model cache dir joined with a name from the compiled-in pinned list, never user input
	in, err := os.Open(path) //nolint:gosec // same
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, in); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadVerified fetches url to dest, hashing as it streams. The file
// takes dest's name only if its size and sha256 match f; otherwise the
// partial file is removed and nothing is left behind.
func downloadVerified(url, dest string, f modelFile) error {
	resp, err := downloadClient.Get(url) //nolint:gosec,noctx // pinned HuggingFace URL; the bytes are hash-checked
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// A temp file of its own: two processes fetching the same file must not
	// write into one another's download, or the hash each checked would not
	// be of the bytes it renames into place.
	out, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := out.Name()
	h := sha256.New()
	written, copyErr := copyWithProgress(io.MultiWriter(out, h), resp.Body, f.local, f.size)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr, check(f, written, hex.EncodeToString(h.Sum(nil)))); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dest)
}

// check compares a download with what the pinned list says it must be.
func check(f modelFile, written int64, sum string) error {
	if written != f.size {
		return fmt.Errorf("got %d bytes, want %d (truncated or wrong file)", written, f.size)
	}
	if sum != f.sha256 {
		return fmt.Errorf("sha256 %s, want %s (the file is not the pinned one)", sum, f.sha256)
	}
	return nil
}

// copyWithProgress copies src to dst, reporting progress on stderr every two
// seconds against the expected size.
func copyWithProgress(dst io.Writer, src io.Reader, name string, size int64) (int64, error) {
	label := humanSize(size)
	fmt.Fprintf(os.Stderr, "  %s (%s): 0%%", name, label)
	var written int64
	lastReport := time.Now()
	buf := make([]byte, 256*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, err
			}
			written += int64(n)
			if time.Since(lastReport) > 2*time.Second {
				fmt.Fprintf(os.Stderr, "\r  %s (%s): %.0f%%", name, label, float64(written)/float64(size)*100)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			fmt.Fprintf(os.Stderr, "\r  %s (%s): done\n", name, label)
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

// humanSize renders a byte count as the download progress shows it.
func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n>>10)
	}
	return fmt.Sprintf("%dB", n)
}
