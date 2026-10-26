package hookscripts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type watchMessage struct {
	ID        string `json:"id"`
	From      string `json:"from_agent"`
	TS        int    `json:"ts"`
	ReceiveTS int    `json:"receive_ts"`
	Room      string `json:"room,omitempty"`
	To        string `json:"to_agent,omitempty"`
	Body      string `json:"body"`
}

func watchMsg(id, from string, ts int, stream string) watchMessage {
	m := watchMessage{ID: id, From: from, TS: ts, ReceiveTS: ts, Body: "hello"}
	if stream == "dm" {
		m.To = "agent:mesh-watch-test"
	} else {
		m.Room = stream
	}
	return m
}

type watchRequest struct {
	Path, Stream string
	Since        int
	HasSince     bool
	At           time.Time
}

// Models wait.rs: stored ts > since returns immediately; otherwise wait for a
// matching later second or timeout. Reads filter/order by receiver timestamp,
// as sqlite_store.rs does; sender timestamps need not be in response order.
type watchDaemon struct {
	mu       sync.Mutex
	messages []watchMessage
	requests []watchRequest
	changed  chan struct{}
	waits    chan watchRequest
	armAdds  map[string][]watchMessage
}

func newWatchDaemon(messages ...watchMessage) *watchDaemon {
	return &watchDaemon{messages: messages, changed: make(chan struct{}), waits: make(chan watchRequest, 100), armAdds: map[string][]watchMessage{}}
}

func (d *watchDaemon) add(messages ...watchMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, messages...)
	close(d.changed)
	d.changed = make(chan struct{})
}

func (d *watchDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, _ := strconv.Atoi(q.Get("since"))
	stream := q.Get("room")
	if stream == "" {
		stream = "dm"
	}
	request := watchRequest{Path: r.URL.Path, Stream: stream, Since: since, HasSince: q.Has("since"), At: time.Now()}
	d.mu.Lock()
	d.requests = append(d.requests, request)
	d.mu.Unlock()
	if r.URL.Path == "/chat_wait" {
		d.waits <- request
	}
	seconds, _ := strconv.Atoi(q.Get("timeout_secs"))
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	for {
		d.mu.Lock()
		messages := []watchMessage{}
		for _, m := range d.messages {
			matches := (q.Get("room") != "" && m.Room == q.Get("room")) || (q.Get("to_agent") != "" && m.To == q.Get("to_agent"))
			if matches && (!request.HasSince || m.ReceiveTS > since) && (r.URL.Path != "/chat_wait" || m.TS > since) {
				messages = append(messages, m)
			}
		}
		sort.SliceStable(messages, func(i, j int) bool { return messages[i].ReceiveTS < messages[j].ReceiveTS })
		changed := d.changed
		additions := d.armAdds[stream]
		if !request.HasSince {
			delete(d.armAdds, stream)
		} else {
			additions = nil
		}
		d.mu.Unlock()
		if len(additions) > 0 {
			d.add(additions...)
		}
		if r.URL.Path == "/chat_read" || len(messages) > 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": messages})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": []watchMessage{}})
			return
		case <-changed:
		}
	}
}

func runWatchDaemon(t *testing.T, d *watchDaemon, ceiling, wait int, inject func(string)) (string, string) {
	t.Helper()
	server := httptest.NewServer(d)
	defer server.Close()
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	dir, bin := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "mesh-watch.sh")
	require.NoError(t, os.WriteFile(script, body, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "LISTEN"), []byte(`{"mode":"all","agents":{"mesh-watch-test":{"rooms":{"mesh":{"scope":"all"},"other":{"scope":"all"}}}}}`), 0o600))
	InstallStub(t, bin, "vaultmind", `
case "$*" in
 'identity paths')
  printf "VM_MESH_SLUG='mesh-watch-test'\nVM_MESH_SELF='agent:mesh-watch-test'\nVM_MESH_OPERATOR=''\nVM_MESH_DAEMON='%s'\nVM_MESH_REGISTRY_DAYS_LEFT=''\n" "$VM_TEST_DAEMON"
  for key in PID LISTEN DISARM HEARTBEAT LASTARM LASTWAKE; do printf 'VM_MESH_%s=%q\n' "$key" "$VM_TEST_ROOT/$key"; done ;;
 'identity fetch-registry') exit 0 ;;
 *) exit 1 ;;
esac
`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ceiling+12)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "AGENT_CHAT_PROJECT_PATH=" + dir, "VM_TEST_ROOT=" + dir, "VM_TEST_DAEMON=" + server.URL, "MESH_WAIT_SECS=" + strconv.Itoa(wait), "MESH_WATCH_MAX_WALL_SECS=" + strconv.Itoa(ceiling)}
	done := make(chan struct{})
	if inject != nil {
		go func() { defer close(done); inject(dir) }()
	} else {
		close(done)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "watcher timeout: %s", out)
	require.NoError(t, err, "%s", out)
	<-done
	return string(out), dir
}

func TestMeshWatch_QuietCallBound(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon(watchMsg("r", "agent:first", 100, "mesh"), watchMsg("d", "agent:first", 100, "dm"), watchMsg("o", "agent:first", 100, "other"))
	out, dir := runWatchDaemon(t, d, 18, 28, nil)
	require.Contains(t, out, "(quiet heartbeat)")
	require.FileExists(t, filepath.Join(dir, "HEARTBEAT"))
	d.mu.Lock()
	defer d.mu.Unlock()
	counts := map[string]int{}
	for _, r := range d.requests {
		counts[r.Path+":"+r.Stream]++
	}
	for _, stream := range []string{"mesh", "other"} {
		assert.LessOrEqual(t, counts["/chat_wait:"+stream], 1, "quiet wait must block for WAIT_SECS: %v", counts)
	}
	for _, stream := range []string{"mesh", "other", "dm"} {
		assert.Equal(t, 3, counts["/chat_read:"+stream], "baseline, arm sweep, and 15s sweep: %v", counts)
	}
}

func TestMeshWatch_EmptyMailboxSinceBound(t *testing.T) {
	t.Parallel()
	d := newWatchDaemon(watchMsg("r", "agent:first", 100, "mesh"), watchMsg("o", "agent:first", 100, "other"))
	runWatchDaemon(t, d, 3, 28, nil)
	d.mu.Lock()
	defer d.mu.Unlock()
	require.NotEmpty(t, d.requests)
	bounded := 0
	for _, r := range d.requests {
		if r.HasSince {
			bounded++
			assert.GreaterOrEqual(t, r.Since, 99, "empty mailbox must not pull history: %+v", r)
		}
	}
	require.Greater(t, bounded, 0, "exercise actual cursor-bearing requests")
}

// The original #350 cases now use the same blocking fake as the call bounds.
func TestMeshWatch_SameSecondCursor(t *testing.T) {
	for _, stream := range []string{"mesh", "other", "dm"} {
		for _, arrival := range []string{"arm", "straggler", "baseline-replay", "poll-replay"} {
			t.Run(stream+"/"+arrival, func(t *testing.T) {
				t.Parallel()
				baselineTS := 100
				if arrival == "straggler" {
					baselineTS = 50
				}
				d := newWatchDaemon(watchMsg("r", "agent:first", baselineTS, "mesh"), watchMsg("d", "agent:first", baselineTS, "dm"), watchMsg("o", "agent:first", baselineTS, "other"))
				next := watchMsg("new", "agent:second", 100, stream)
				var inject func(string)
				var added time.Time
				stragglerAfterPoll := false
				replayUnmuted := false
				if arrival == "baseline-replay" {
					// Baseline IDs replayed by overlap must stay quiet.
				} else if arrival == "poll-replay" {
					inject = func(dir string) {
						listen := filepath.Join(dir, "LISTEN")
						<-d.waits
						assert.NoError(t, os.WriteFile(listen+".tmp", []byte(`{"mode":"except","mute":["agent:second"]}`), 0o600))
						assert.NoError(t, os.Rename(listen+".tmp", listen))
						next.TS, next.ReceiveTS = 101, 101
						d.add(next)
						// Wait for the target worker to re-arm after observing
						// this ID while muted, then allow the same stored ID.
						for {
							var r watchRequest
							select {
							case r = <-d.waits:
							case <-time.After(8 * time.Second):
								return
							}
							target := stream
							if target == "dm" {
								target = "mesh"
							}
							if r.Stream == target && r.Since == 101 {
								break
							}
						}
						assert.NoError(t, os.WriteFile(listen+".tmp", []byte(`{"mode":"all"}`), 0o600))
						assert.NoError(t, os.Rename(listen+".tmp", listen))
						replayUnmuted = true
					}
				} else if arrival == "arm" {
					d.armAdds[stream] = []watchMessage{next}
				} else {
					inject = func(_ string) {
						<-d.waits
						d.add(watchMsg("self", "agent:mesh-watch-test", 100, stream))
						// The first later-second message advances the cursor
						// through an actual wait return before the straggler.
						for {
							var r watchRequest
							select {
							case r = <-d.waits:
							case <-time.After(8 * time.Second):
								return
							}
							target := stream
							if target == "dm" {
								target = "mesh"
							}
							if r.Stream == target && r.Since == 100 {
								break
							}
						}
						stragglerAfterPoll = true
						time.Sleep(time.Second)
						added = time.Now()
						d.add(next)
					}
				}
				ceiling, wait := 21, 28
				if arrival == "baseline-replay" || arrival == "poll-replay" {
					ceiling, wait = 8, 1
				}
				out, dir := runWatchDaemon(t, d, ceiling, wait, inject)
				if arrival == "baseline-replay" || arrival == "poll-replay" {
					require.NotContains(t, out, "WAKE:")
					require.Contains(t, out, "(quiet heartbeat)")
					require.NoFileExists(t, filepath.Join(dir, "LASTWAKE"))
					if arrival == "poll-replay" {
						require.True(t, replayUnmuted, "re-enable listening after the ID was observed")
					}
					return
				}
				require.Contains(t, out, "message from agent:second (ts 100)")
				require.Contains(t, out, "then RE-ARM mesh-watch")
				wake, err := os.ReadFile(filepath.Join(dir, "LASTWAKE"))
				require.NoError(t, err)
				require.Equal(t, "agent:second|100", string(wake))
				if arrival == "straggler" {
					require.True(t, stragglerAfterPoll, "straggler follows an observed poll, not only the baseline")
					info, statErr := os.Stat(filepath.Join(dir, "LASTWAKE"))
					require.NoError(t, statErr)
					lag := info.ModTime().Sub(added)
					t.Logf("straggler recorded after %s", lag)
					// The sweep fires when bash's whole-second SECONDS has advanced 15, checked
					// once per loop tick, so the design's worst case is ~17s before the read
					// itself; 16s flaked under `task check` load (16.37s). 20s still fails a
					// watcher that misses the 15s sweep and waits for the next one (30s).
					require.LessOrEqual(t, lag, 20*time.Second, "one 15s sweep interval plus whole-second rounding, a loop tick and the read")
				}
			})
		}
	}
}
