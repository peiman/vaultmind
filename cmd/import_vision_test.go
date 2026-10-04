package cmd

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImport_VisionFlagsDescribeImagesWithTheKeyFromTheNamedVariable(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no tesseract: the description is the only text
	t.Setenv("MY_VISION_KEY", "sk-from-env")
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "A blue square."}}}})
	}))
	t.Cleanup(srv.Close)
	vault := indexedBaselineVault(t)
	dir := filepath.Join(t.TempDir(), "pics")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 300, 300))))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "square.png"), buf.Bytes(), 0o600))

	out, errOut, err := runRootCmd(t, "import", dir, "--vault", vault,
		"--vision-endpoint", srv.URL+"/v1", "--vision-model", "gemma4:31b", "--vision-api-key-env", "MY_VISION_KEY")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "1 added")
	assert.Equal(t, "Bearer sk-from-env", auth)
	assert.NotContains(t, errOut.String(), "sent to", "a local endpoint is this machine")
}

func TestImport_AVisionEndpointNeedsAModel(t *testing.T) {
	vault := indexedBaselineVault(t)
	_, _, err := runRootCmd(t, "import", t.TempDir(), "--vault", vault, "--vision-endpoint", "http://localhost:11434/v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--vision-model")
}

func TestVisionLeavesTheMachine(t *testing.T) {
	got := map[string]bool{}
	for _, e := range []string{"http://localhost:11434/v1", "http://127.0.0.1:1234/v1", "http://[::1]:8080/v1", "https://api.openai.com/v1", "http://gpu-box.lan:11434/v1"} {
		got[e] = visionLeavesTheMachine(e)
	}
	assert.Equal(t, map[string]bool{"http://localhost:11434/v1": false, "http://127.0.0.1:1234/v1": false, "http://[::1]:8080/v1": false,
		"https://api.openai.com/v1": true, "http://gpu-box.lan:11434/v1": true}, got)
}

func TestImport_AnUnsetKeyVariableIsNamed(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("MISSING_VISION_KEY", "")
	vault := indexedBaselineVault(t)
	dir := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o600))
	_, errOut, err := runRootCmd(t, "import", dir, "--vault", vault,
		"--vision-endpoint", "http://localhost:1/v1", "--vision-model", "m", "--vision-api-key-env", "MISSING_VISION_KEY")
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "MISSING_VISION_KEY is not set")
}
