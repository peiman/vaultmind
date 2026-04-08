package embedding

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

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

	for _, f := range bgem3Files {
		localPath := filepath.Join(modelDir, f.local)
		if _, err := os.Stat(localPath); err == nil {
			continue
		}
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", bgem3Repo, f.remote)
		fmt.Fprintf(os.Stderr, "Downloading %s (%s)...\n", f.local, f.size)
		if err := downloadFile(url, localPath); err != nil {
			return "", fmt.Errorf("downloading %s: %w", f.local, err)
		}
	}

	return modelDir, nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec,noctx // trusted HuggingFace URL
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

	_, err = io.Copy(out, resp.Body)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	return os.Rename(tmpPath, dest)
}
