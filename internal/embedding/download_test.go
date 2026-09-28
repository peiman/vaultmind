package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modelServer serves content by remote path and counts requests, so a test
// can tell a download from a cache hit.
type modelServer struct {
	*httptest.Server
	mu       sync.Mutex
	content  map[string]string
	requests map[string]int
}

func newModelServer(t *testing.T, content map[string]string) *modelServer {
	t.Helper()
	s := &modelServer{content: content, requests: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests[r.URL.Path]++
		body, ok := s.content[strings.TrimPrefix(r.URL.Path, "/")]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *modelServer) count(remote string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests["/"+remote]
}

func fileFor(remote, local, body string) modelFile {
	sum := sha256.Sum256([]byte(body))
	return modelFile{remote: remote, local: local, size: int64(len(body)), sha256: hex.EncodeToString(sum[:])}
}

var testFiles = []modelFile{
	fileFor("onnx/model.onnx", "model.onnx", "the model"),
	fileFor("config.json", "config.json", `{"a":1}`),
}

func testContent() map[string]string {
	return map[string]string{"onnx/model.onnx": "the model", "config.json": `{"a":1}`}
}

func TestEnsureModel_DownloadsAndVerifiesEachFile(t *testing.T) {
	srv, dir := newModelServer(t, testContent()), t.TempDir()

	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))
	got, err := os.ReadFile(filepath.Join(dir, "model.onnx")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "the model", string(got))
	assert.FileExists(t, filepath.Join(dir, ".verified-rev1"))
}

// A file whose bytes do not match the pinned hash — substituted, or
// corrupted in transit — is refused and never takes the file's name.
func TestEnsureModel_RefusesContentThatDoesNotMatchTheHash(t *testing.T) {
	content := testContent()
	content["onnx/model.onnx"] = "the modeX" // same length, different bytes
	srv, dir := newModelServer(t, content), t.TempDir()

	err := ensureModel(srv.URL, dir, "rev1", testFiles)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sha256")
	assert.NoFileExists(t, filepath.Join(dir, "model.onnx"))
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	assert.Empty(t, leftovers, "the partial download is removed")
	assert.NoFileExists(t, filepath.Join(dir, ".verified-rev1"))
}

// A truncated download that still ended with HTTP 200 is refused.
func TestEnsureModel_RefusesATruncatedFile(t *testing.T) {
	content := testContent()
	content["onnx/model.onnx"] = "the mo"
	srv, dir := newModelServer(t, content), t.TempDir()

	err := ensureModel(srv.URL, dir, "rev1", testFiles)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bytes")
	assert.NoFileExists(t, filepath.Join(dir, "model.onnx"))
}

// A cache from before verification existed is hashed once: good files are
// kept without a download, and the marker records that they were checked.
func TestEnsureModel_VerifiesAnExistingCacheOnceWithoutDownloading(t *testing.T) {
	srv, dir := newModelServer(t, testContent()), t.TempDir()
	for remote, body := range testContent() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.Base(remote)), []byte(body), 0o600))
	}

	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))
	assert.Zero(t, srv.count("onnx/model.onnx"))
	assert.Zero(t, srv.count("config.json"))
	assert.FileExists(t, filepath.Join(dir, ".verified-rev1"))
}

// A cached file that fails the hash is replaced by a verified download.
func TestEnsureModel_ReplacesACachedFileThatFailsTheHash(t *testing.T) {
	srv, dir := newModelServer(t, testContent()), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.onnx"), []byte("tampered!"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"a":1}`), 0o600))

	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))
	assert.Equal(t, 1, srv.count("onnx/model.onnx"))
	assert.Zero(t, srv.count("config.json"), "the good file is kept")
	got, _ := os.ReadFile(filepath.Join(dir, "model.onnx")) //nolint:gosec // test path
	assert.Equal(t, "the model", string(got))
}

// Once verified, a file whose size changes is checked again.
func TestEnsureModel_RechecksAVerifiedFileWhoseSizeChanged(t *testing.T) {
	srv, dir := newModelServer(t, testContent()), t.TempDir()
	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.onnx"), []byte("cut"), 0o600))

	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))
	assert.Equal(t, 2, srv.count("onnx/model.onnx"))
	got, _ := os.ReadFile(filepath.Join(dir, "model.onnx")) //nolint:gosec // test path
	assert.Equal(t, "the model", string(got))
}

// A new pinned revision verifies the cache again: the marker is per revision.
func TestEnsureModel_ANewRevisionVerifiesAgain(t *testing.T) {
	srv, dir := newModelServer(t, testContent()), t.TempDir()
	require.NoError(t, ensureModel(srv.URL, dir, "rev1", testFiles))

	require.NoError(t, ensureModel(srv.URL, dir, "rev2", testFiles))
	assert.FileExists(t, filepath.Join(dir, ".verified-rev2"))
}

// Every model VaultMind downloads is pinned: a full commit, and a size and
// hash for every file. A model missing from the table is not downloaded.
func TestPinnedModels_ArePinned(t *testing.T) {
	for _, name := range []string{DefaultModelName, BGEM3ModelName} {
		m, ok := pinnedModels[name]
		require.True(t, ok, "%s has no pinned files", name)
		assert.Len(t, m.revision, 40, "%s: a full commit sha, not a branch", name)
		require.NotEmpty(t, m.files, name)
		for _, f := range m.files {
			assert.Positive(t, f.size, "%s %s", name, f.local)
			assert.Len(t, f.sha256, 64, "%s %s", name, f.local)
		}
	}
}

// A model with no pinned files is refused rather than fetched unchecked.
func TestDownloadPinned_RefusesAModelWithNoPinnedFiles(t *testing.T) {
	_, err := downloadPinned(t.TempDir(), "someone/unknown-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pinned files")
}

// Pinned models land where the embedding pipelines read them: the layout
// hugot used, <cache>/<owner>_<name>.
func TestDownloadPinned_UsesTheModelDirectoryThePipelineReads(t *testing.T) {
	content := map[string]string{}
	for remote, body := range testContent() {
		content["owner/name/resolve/rev1/"+remote] = body // the URL shape HuggingFace serves
	}
	srv, cache := newModelServer(t, content), t.TempDir()
	saved := pinnedModels
	pinnedModels = map[string]pinnedModel{"owner/name": {repo: "owner/name", revision: "rev1", files: testFiles}}
	savedBase := huggingFaceBase
	huggingFaceBase = srv.URL
	t.Cleanup(func() { pinnedModels, huggingFaceBase = saved, savedBase })

	dir, err := downloadPinned(cache, "owner/name")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cache, "owner_name"), dir)
	assert.FileExists(t, filepath.Join(dir, "model.onnx"))
}
