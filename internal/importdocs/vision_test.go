package importdocs_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// visionServer is a fake OpenAI-compatible endpoint answering every image
// with reply, recording each request it gets.
type visionServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	auth     []string
}

func newVisionServer(t *testing.T, reply string) *visionServer {
	t.Helper()
	v := &visionServer{}
	v.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		v.mu.Lock()
		v.requests = append(v.requests, body)
		v.auth = append(v.auth, r.Header.Get("Authorization"))
		v.mu.Unlock()
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": reply, "reasoning": "thinking about it"}}}})
	}))
	t.Cleanup(v.Close)
	return v
}

func (v *visionServer) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.requests)
}

func TestImport_WithoutAVisionEndpointNoImageLeavesTheMachine(t *testing.T) {
	fakeTesseract(t, tsvWords(map[string]int{"Ingest": 91}, []string{"Ingest"}))
	v := newVisionServer(t, "A diagram.")
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "flow.png"), string(pngBytes(t, 640, 360)))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Zero(t, v.count(), "vision is opt-in")
	assert.NotContains(t, skipReasons(res), "not described", "no description is even attempted")
	_, body := noteAt(t, vault, "imported/demo-repo/docs/flow-png.md")
	assert.NotContains(t, body, "## Description")
}

func TestImport_AVisionModelDescribesAnImageOnceAndTheCacheServesTheRest(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	t.Setenv("TEST_VISION_KEY", "sk-test-123")
	v := newVisionServer(t, "A flowchart of the ingest pipeline: source, parse, note.")
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "flow.png"), string(pngBytes(t, 640, 360)))
	write(t, filepath.Join(repo, "docs", "design.md"), "# Design\n\n![The ingest flow](flow.png)\n")
	opts := importdocs.Options{Vision: importdocs.Vision{Endpoint: v.URL + "/v1", Model: "gemma4:31b", APIKey: "sk-test-123"}}

	run(t, repo, vault, opts)
	require.Equal(t, 1, v.count())
	_, body := noteAt(t, vault, "imported/demo-repo/docs/flow-png.md")
	assert.Contains(t, body, "## Description\n\nA flowchart of the ingest pipeline: source, parse, note.")
	assert.NotContains(t, body, "thinking about it", "a reasoning model's thoughts are not the description")
	req := v.requests[0]
	assert.Equal(t, "gemma4:31b", req["model"])
	assert.Equal(t, "Bearer sk-test-123", v.auth[0])
	raw, _ := json.Marshal(req["messages"])
	assert.Contains(t, string(raw), "data:image/png;base64,")
	assert.Contains(t, string(raw), "The ingest flow", "the caption the docs give it is context")

	run(t, repo, vault, opts)
	assert.Equal(t, 1, v.count(), "a re-import is described from the cache")
}

func TestImport_AVisionFailureLeavesTheNoteAndSaysWhy(t *testing.T) {
	fakeTesseract(t, tsvWords(map[string]int{"Ingest": 91}, []string{"Ingest"}))
	v := newVisionServer(t, "") // a reasoning model that spent its budget
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "flow.png"), string(pngBytes(t, 640, 360)))

	res := run(t, repo, vault, importdocs.Options{Vision: importdocs.Vision{Endpoint: v.URL + "/v1", Model: "m"}})
	_, body := noteAt(t, vault, "imported/demo-repo/docs/flow-png.md")
	assert.Contains(t, body, "Ingest")
	assert.NotContains(t, body, "## Description")
	reasons := skipReasons(res)
	assert.Contains(t, reasons, "1 image(s) — not described")
	assert.Contains(t, reasons, "no description")
	assert.Equal(t, "", v.auth[0], "no key configured, no Authorization header")
}

func TestImport_IconsAndHEICAreNotSentForDescription(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	v := newVisionServer(t, "An icon.")
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "icon.png"), string(pngBytes(t, 32, 32)))
	run(t, repo, vault, importdocs.Options{Vision: importdocs.Vision{Endpoint: v.URL + "/v1", Model: "m"}})
	assert.Zero(t, v.count())
	_ = strings.TrimSpace
}
