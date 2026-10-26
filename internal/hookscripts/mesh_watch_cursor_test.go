package hookscripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type watchCursor struct {
	TS  int      `json:"ts"`
	IDs []string `json:"ids"`
}

func cursorFile(t *testing.T, initial string) (string, map[string]string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cursor")
	require.NoError(t, os.WriteFile(path, []byte(initial), 0o600))
	return path, detectorEnv(t, map[string]string{"VM_WAKE_CURSOR_FILE": path})
}

func readWatchCursors(t *testing.T, path string) map[string]watchCursor {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var cursors map[string]watchCursor
	require.NoError(t, json.Unmarshal(data, &cursors))
	return cursors
}

func TestMeshWatchDetector_ResponseSnapshot(t *testing.T) {
	for _, sender := range []string{"agent:me", "agent:muted"} {
		t.Run(sender, func(t *testing.T) {
			path, env := cursorFile(t, `{"room":{"ts":99,"ids":["old"]}}`)
			listen := filepath.Join(t.TempDir(), "listen")
			require.NoError(t, os.WriteFile(listen, []byte(`{"mode":"except","mute":["agent:muted"]}`), 0o600))
			env["VM_WAKE_LISTEN_FILE"] = listen
			out, rc := runDetector(t, env, `{"messages":[{"id":"B","from_agent":"`+sender+`","ts":101,"room":"mesh"},{"id":"A","from_agent":"agent:other","ts":100,"room":"mesh"}]}`)
			require.Zero(t, rc)
			require.Equal(t, "agent:other|100", out)
			require.Equal(t, watchCursor{TS: 101, IDs: []string{"B"}}, readWatchCursors(t, path)["room"])
		})
	}
}

func TestMeshWatchDetector_BoundedSeenIDs(t *testing.T) {
	path, env := cursorFile(t, `{"room":{"ts":100,"ids":["old-a","old-b"]}}`)
	// Duplicate IDs in one batch must be a set; advancing must discard old IDs.
	out, rc := runDetector(t, env, `{"messages":[{"id":"new","from_agent":"agent:me","ts":101,"room":"mesh"},{"id":"new","from_agent":"agent:me","ts":101,"room":"mesh"},{"id":"older","from_agent":"agent:other","ts":100,"room":"mesh"}]}`)
	require.Zero(t, rc)
	require.Equal(t, "agent:other|100", out, "dropping old IDs must not drop an unseen message in the same response")
	require.Equal(t, watchCursor{TS: 101, IDs: []string{"new"}}, readWatchCursors(t, path)["room"])
}

func TestMeshWatchDetector_MissingIDsAndDescendingBaseline(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)
	start := strings.Index(s, "newest_cursor() {")
	require.Positive(t, start)
	end := strings.Index(s[start:], "\n}\n")
	require.Positive(t, end)
	bin := t.TempDir()
	InstallStub(t, bin, "curl", `printf '%s' '{"messages":[{"from_agent":"agent:first","ts":101,"room":"mesh","body":"first"},{"id":"older","ts":100}]}'`)
	cmd := exec.Command("bash", "-c", s[start:start+end+3]+"\nnewest_cursor unused")
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.Output()
	require.NoError(t, err)
	var baseline watchCursor
	require.NoError(t, json.Unmarshal(out, &baseline))
	require.Equal(t, 101, baseline.TS, "sender timestamp max, independent of response order")
	require.Len(t, baseline.IDs, 1)
	require.NotEmpty(t, baseline.IDs[0], "baseline missing IDs need stable identities too")
	path, env := cursorFile(t, `{"room":`+strings.TrimSpace(string(out))+`}`)
	replay := `{"messages":[{"from_agent":"agent:first","ts":101,"room":"mesh","body":"first"}]}`
	hit, rc := runDetector(t, env, replay)
	require.Zero(t, rc)
	require.Empty(t, hit, "baseline fallback identity must agree with the detector")
	hit, rc = runDetector(t, env, `{"messages":[{"from_agent":"agent:first","ts":101,"room":"mesh","body":"different"}]}`)
	require.Zero(t, rc)
	require.Equal(t, "agent:first|101", hit, "same sender and second, different body is new")
	require.Len(t, readWatchCursors(t, path)["room"].IDs, 2)
	hit, rc = runDetector(t, env, replay)
	require.Zero(t, rc)
	require.Empty(t, hit, "fallback ID must stay stable across invocations")
}

func TestMeshWatchDetector_AtomicCursor(t *testing.T) {
	path, env := cursorFile(t, `{"room":{"ts":100,"ids":[]}}`)
	// An open reader keeps the old complete generation after a rename. A direct
	// truncating write changes what this reader sees and fails this assertion.
	reader, err := os.Open(path)
	require.NoError(t, err)
	defer reader.Close()
	out, rc := runDetector(t, env, `{"messages":[{"id":"new","from_agent":"agent:me","ts":101,"room":"mesh"}]}`)
	require.Zero(t, rc)
	require.Empty(t, out)
	old := make([]byte, 200)
	n, err := reader.Read(old)
	require.NoError(t, err)
	require.JSONEq(t, `{"room":{"ts":100,"ids":[]}}`, string(old[:n]))
	require.Equal(t, 101, readWatchCursors(t, path)["room"].TS)
}
