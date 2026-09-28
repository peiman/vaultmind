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

const (
	huggingFaceBase = "https://huggingface.co"
	bgem3Repo       = "BAAI/bge-m3"
	// bgem3Revision pins the commit the files below come from. `main` moves;
	// a pinned commit and a hash per file mean the bytes we run are the bytes
	// we checked (#192, review M7).
	bgem3Revision = "5617a9f61b028005a4858fdac845db406aefb181"
)

// modelFile is one file of a pinned model and what it must be.
type modelFile struct {
	remote string
	local  string
	size   int64
	sha256 string
}

// bgem3Files are the BGE-M3 files at bgem3Revision. Sizes and hashes are
// HuggingFace's LFS records for that commit, or the sha256 of the served
// bytes for the three small files LFS does not track; all eight were checked
// against a cache in use.
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
	modelDir := filepath.Join(cacheDir, "BAAI_bge-m3")
	base := fmt.Sprintf("%s/%s/resolve/%s", huggingFaceBase, bgem3Repo, bgem3Revision)
	if err := ensureModel(base, modelDir, bgem3Revision, bgem3Files); err != nil {
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
			fmt.Fprintf(os.Stderr, "BGE-M3 model files missing or not verified. Downloading to %s\n", modelDir)
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

	tmpPath := dest + ".tmp"
	out, err := os.Create(tmpPath) //nolint:gosec // path in the model cache, named by the pinned list
	if err != nil {
		return err
	}
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
