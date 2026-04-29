//go:build dev

package embedding_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/embedding"
)

func TestSidecar_Speed_SingleVsBatched(t *testing.T) {
	if os.Getenv("VAULTMIND_SIDECAR_BENCH") == "" {
		t.Skip("set VAULTMIND_SIDECAR_BENCH=1")
	}

	// Read 8 real note bodies — same workload as throughput bench
	const N = 8
	vaultDir := "../../vaultmind-identity"
	var texts []string
	for _, root := range []string{filepath.Join(vaultDir, "arcs"), filepath.Join(vaultDir, "references")} {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if len(texts) >= N {
				break
			}
			if e.IsDir() {
				continue
			}
			body, _ := os.ReadFile(filepath.Join(root, e.Name()))
			texts = append(texts, string(body))
		}
	}

	scriptPath, _ := filepath.Abs(filepath.Join("sidecar", "embed_server.py"))
	side, err := embedding.NewSidecarBGEM3(embedding.SidecarBGEM3Config{
		Python:     os.Getenv("VAULTMIND_SIDECAR_PYTHON"),
		ScriptPath: scriptPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer side.Close()

	// One batch of 8
	start := time.Now()
	_, err = side.EmbedFullBatch(context.Background(), texts)
	batchedElapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	// 8 separate batches of 1
	start = time.Now()
	for _, txt := range texts {
		_, err = side.EmbedFullBatch(context.Background(), []string{txt})
		if err != nil {
			t.Fatal(err)
		}
	}
	singletonElapsed := time.Since(start)

	t.Log(fmt.Sprintf("batched (8 in 1):    %v (%.3fs/note)", batchedElapsed, batchedElapsed.Seconds()/float64(N)))
	t.Log(fmt.Sprintf("singletons (8 calls): %v (%.3fs/note)", singletonElapsed, singletonElapsed.Seconds()/float64(N)))
	t.Log(fmt.Sprintf("ratio singleton/batched: %.2fx slower", float64(singletonElapsed)/float64(batchedElapsed)))
}
