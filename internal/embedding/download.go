package embedding

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// downloadClient is an HTTP client with a generous timeout for large model downloads.
var downloadClient = &http.Client{Timeout: 30 * time.Minute}

const bgem3Repo = "BAAI/bge-m3"

var bgem3Files = []struct {
	remote string
	local  string
	size   string
}{
	{"onnx/model.onnx", "model.onnx", "725KB"},
	{"onnx/model.onnx_data", "model.onnx_data", "2.1GB"},
	{"tokenizer.json", "tokenizer.json", "17MB"},
	{"tokenizer_config.json", "tokenizer_config.json", "1KB"},
	{"special_tokens_map.json", "special_tokens_map.json", "1KB"},
	{"config.json", "config.json", "1KB"},
	{"sparse_linear.pt", "sparse_linear.pt", "4KB"},
	{"colbert_linear.pt", "colbert_linear.pt", "2.1MB"},
}

// DownloadBGEM3 downloads BGE-M3 model files from HuggingFace if not already cached.
// Returns the path to the model directory.
func DownloadBGEM3(cacheDir string) (string, error) {
	modelDir := filepath.Join(cacheDir, "BAAI_bge-m3")
	if err := os.MkdirAll(modelDir, 0o750); err != nil {
		return "", fmt.Errorf("creating model directory: %w", err)
	}

	// Check if all files already exist
	allPresent := true
	for _, f := range bgem3Files {
		if _, err := os.Stat(filepath.Join(modelDir, f.local)); err != nil {
			allPresent = false
			break
		}
	}
	if allPresent {
		return modelDir, nil
	}

	fmt.Fprintf(os.Stderr, "BGE-M3 model not cached. Downloading ~2.2GB to %s\n", modelDir)

	for _, f := range bgem3Files {
		localPath := filepath.Join(modelDir, f.local)
		if _, err := os.Stat(localPath); err == nil {
			continue
		}
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", bgem3Repo, f.remote)
		if err := downloadFileWithProgress(url, localPath, f.local, f.size); err != nil {
			return "", fmt.Errorf("downloading %s: %w", f.local, err)
		}
	}

	fmt.Fprintf(os.Stderr, "BGE-M3 model ready.\n")
	return modelDir, nil
}

func downloadFileWithProgress(url, dest, name, sizeLabel string) error {
	resp, err := downloadClient.Get(url) //nolint:gosec,noctx // trusted HuggingFace URL
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	tmpPath := dest + ".tmp"
	out, err := os.Create(tmpPath) //nolint:gosec // path constructed from trusted config
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var written int64
	lastReport := time.Now()

	// Print initial progress line
	if total > 0 {
		fmt.Fprintf(os.Stderr, "  %s (%s): 0%%", name, sizeLabel)
	} else {
		fmt.Fprintf(os.Stderr, "  %s (%s): downloading...", name, sizeLabel)
	}

	buf := make([]byte, 256*1024) // 256KB chunks
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return writeErr
			}
			written += int64(n)

			// Update progress every 2 seconds
			if total > 0 && time.Since(lastReport) > 2*time.Second {
				pct := float64(written) / float64(total) * 100
				fmt.Fprintf(os.Stderr, "\r  %s (%s): %.0f%%", name, sizeLabel, pct)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return readErr
		}
	}

	fmt.Fprintf(os.Stderr, "\r  %s (%s): done\n", name, sizeLabel)

	closeErr := out.Close()
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	return os.Rename(tmpPath, dest)
}
