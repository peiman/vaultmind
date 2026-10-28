package hookscripts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../plugin")
	require.NoError(t, err)
	return p
}

func pluginRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(pluginRoot(t), path))
	require.NoError(t, err)
	return b
}

func TestPluginAssetsStayInSync(t *testing.T) {
	for _, name := range hooks.ScriptsForProfile(hooks.ProfileKnowledge) {
		b, ok := hookscripts.Get(name)
		require.True(t, ok)
		assert.True(t, bytes.Equal(b, pluginRead(t, "scripts/"+name)), "copy internal/hookscripts/%s into plugin/scripts", name)
		dogfood, err := os.ReadFile(filepath.Join("../../.claude/scripts", name))
		require.NoError(t, err)
		assert.Equal(t, b, dogfood, "copy internal/hookscripts/%s into .claude/scripts", name)
	}
	var got, want map[string]any
	require.NoError(t, json.Unmarshal(pluginRead(t, "hooks/hooks.json"), &got))
	canonical, err := hooks.SettingsStanzaForProfile(hooks.ProfileKnowledge, "")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(canonical), &want))
	for _, groups := range want["hooks"].(map[string]any) {
		for _, group := range groups.([]any) {
			for _, item := range group.(map[string]any)["hooks"].([]any) {
				h := item.(map[string]any)
				name := filepath.Base(h["command"].(string))
				h["command"] = `PATH="${CLAUDE_PLUGIN_ROOT}/bin:$PATH" VAULTMIND_HOOK_SOURCE=plugin bash "${CLAUDE_PLUGIN_ROOT}/scripts/` + name + `"`
			}
		}
	}
	assert.Equal(t, want, got)
	skill := string(pluginRead(t, "skills/vaultmind-setup/SKILL.md"))
	commands := append([]string{"vaultmind init " + onboard.QuickstartVault}, onboard.FirstSteps(onboard.QuickstartVault)[:3]...)
	_, quickstart, ok := strings.Cut(skill, "Run these canonical Quickstart steps")
	require.True(t, ok)
	_, block, ok := strings.Cut(quickstart, "```bash\n")
	require.True(t, ok)
	block, _, ok = strings.Cut(block, "```")
	require.True(t, ok)
	assert.Equal(t, strings.Join(commands, "\n")+"\n", block)
	assert.Contains(t, skill, onboard.ShellExpandSentence)
}

func pluginVersion(t *testing.T) string {
	t.Helper()
	var manifest struct{ Version string }
	require.NoError(t, json.Unmarshal(pluginRead(t, ".claude-plugin/plugin.json"), &manifest))
	require.NotEmpty(t, manifest.Version)
	return manifest.Version
}

func TestPluginVersionTracksReleaseIntent(t *testing.T) {
	var manifest struct{ Version string }
	require.NoError(t, json.Unmarshal(pluginRead(t, ".claude-plugin/plugin.json"), &manifest))
	changelog, err := os.ReadFile("../../CHANGELOG.md")
	require.NoError(t, err)
	// The Unreleased heading declares the release PR's intended version until
	// that release gets its dated heading. No second version file to update.
	headings := strings.Split(string(changelog), "\n## ")
	require.GreaterOrEqual(t, len(headings), 3)
	if strings.Contains(headings[1], "Plugin release: `") {
		_, intent, _ := strings.Cut(headings[1], "Plugin release: `")
		version, _, _ := strings.Cut(intent, "`")
		assert.Equal(t, version, manifest.Version)
	} else {
		version, _, ok := strings.Cut(strings.TrimPrefix(headings[2], "["), "]")
		require.True(t, ok)
		assert.Equal(t, version, manifest.Version)
	}
}

func fakeRelease(t *testing.T, osname, arch string, corrupt bool) (string, *atomic.Int32) {
	t.Helper()
	return fakeReleaseWith(t, osname, arch, corrupt, nil)
}

// fakeReleaseWith runs before (when set) ahead of serving each request.
func fakeReleaseWith(t *testing.T, osname, arch string, corrupt bool, before func(*http.Request)) (string, *atomic.Int32) {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	prefix := ""
	checksum := "checksums.txt"
	filename := fmt.Sprintf("vaultmind_%s_%s_%s.tar.gz", pluginVersion(t), osname, arch)
	if osname+"_"+arch == "darwin_arm64" || osname+"_"+arch == "linux_amd64" {
		prefix = "vaultmind_v" + pluginVersion(t) + "_" + osname + "_" + arch + "_ort/"
		filename = strings.TrimSuffix(prefix, "/") + ".tar.gz"
		checksum = "checksums-ort.txt"
	}
	for name, content := range map[string]string{"vaultmind": "#!/bin/sh\nprintf 'fake:%s\\n' \"$*\"\nprintf 'cache:%s\\n' \"$XDG_CACHE_HOME\" >&2\n", "libonnxruntime.so": "fake library"} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: prefix + name, Mode: 0755, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	sum := sha256.Sum256(archive.Bytes())
	digest := hex.EncodeToString(sum[:])
	if corrupt {
		digest = strings.Repeat("0", 64)
	}
	releaseDir := t.TempDir()
	versionDir := filepath.Join(releaseDir, "v"+pluginVersion(t))
	require.NoError(t, os.MkdirAll(versionDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, filename), archive.Bytes(), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, checksum), []byte(fmt.Sprintf("%s  %s\n", digest, filename)), 0644))
	requests := new(atomic.Int32)
	files := http.FileServer(http.Dir(releaseDir))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if before != nil {
			before(r)
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL, requests
}

func wrapperRun(t *testing.T, env []string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", append([]string{filepath.Join(pluginRoot(t), "bin/vaultmind")}, args...)...)
	c.WaitDelay = time.Second
	c.Env = append(os.Environ(), env...)
	var out, stderr bytes.Buffer
	c.Stdout, c.Stderr = &out, &stderr
	err := c.Run()
	return out.String(), stderr.String(), err
}

func TestPluginWrapperDownloadAndCache(t *testing.T) {
	for _, platform := range []struct{ os, arch string }{{"Darwin", "arm64"}, {"Linux", "x86_64"}, {"Linux", "aarch64"}, {"Darwin", "x86_64"}} {
		t.Run(platform.os+platform.arch, func(t *testing.T) {
			tools := t.TempDir()
			hookscripts.InstallStub(t, tools, "uname", "#!/bin/sh\ncase $1 in -s) echo "+platform.os+";; -m) echo "+platform.arch+";; esac\n")
			osname := strings.ToLower(platform.os)
			arch := platform.arch
			if arch == "x86_64" {
				arch = "amd64"
			}
			if arch == "aarch64" {
				arch = "arm64"
			}
			url, requests := fakeRelease(t, osname, arch, false)
			data := filepath.Join(t.TempDir(), "data with spaces")
			env := []string{"PATH=" + tools + ":/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT=" + pluginRoot(t), "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "XDG_CACHE_HOME=inherited-model-cache"}
			out, stderr, err := wrapperRun(t, env, "ask", "two words")
			require.NoError(t, err, stderr)
			assert.Equal(t, "fake:ask two words\n", out)
			assert.Contains(t, stderr, "cache:inherited-model-cache\n")
			assert.FileExists(t, filepath.Join(data, pluginVersion(t), "libonnxruntime.so"))
			count := requests.Load()
			out, stderr, err = wrapperRun(t, env, "mcp")
			require.NoError(t, err, stderr)
			assert.Equal(t, "fake:mcp\n", out)
			assert.Equal(t, count, requests.Load(), "cached binary must not download again")
		})
	}
}

func TestPluginWrapperRefusesChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper")
	}
	url, _ := fakeRelease(t, runtime.GOOS, runtime.GOARCH, true)
	data := t.TempDir()
	out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT=" + pluginRoot(t), "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "XDG_CACHE_HOME=inherited-model-cache"})
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "checksum mismatch")
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t), "vaultmind"))
}

func TestPluginWrapperUsesMatchingPATHBinary(t *testing.T) {
	for _, version := range []string{"v" + pluginVersion(t), "v" + pluginVersion(t) + "0"} {
		t.Run(version, func(t *testing.T) {
			tools := t.TempDir()
			data := t.TempDir()
			hookscripts.InstallStub(t, tools, "vaultmind", "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'vaultmind version "+version+", commit abc'; else echo system; fi\n")
			cached := filepath.Join(data, pluginVersion(t))
			require.NoError(t, os.MkdirAll(cached, 0755))
			hookscripts.InstallStub(t, cached, "vaultmind", "#!/bin/sh\necho cached\n")
			out, stderr, err := wrapperRun(t, []string{"PATH=" + filepath.Join(pluginRoot(t), "bin") + ":" + tools + ":/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT=" + pluginRoot(t), "CLAUDE_PLUGIN_DATA=" + data}, "ask")
			require.NoError(t, err, stderr)
			if version == "v"+pluginVersion(t) {
				assert.Equal(t, "system\n", out)
			} else {
				assert.Equal(t, "cached\n", out)
			}
		})
	}
}

func TestPluginSessionStartAndCoexistence(t *testing.T) {
	var manifest struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	require.NoError(t, json.Unmarshal(pluginRead(t, "hooks/hooks.json"), &manifest))
	command := manifest.Hooks["SessionStart"][0].Hooks[0].Command
	project := t.TempDir()
	tools := t.TempDir()
	home := t.TempDir()
	hookscripts.InstallStub(t, tools, "vaultmind", "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'vaultmind version v"+pluginVersion(t)+"'; elif [ \"$1\" = doctor ]; then echo '{\"result\":{\"embedding_tier\":\"bge-m3\"}}'; fi\n")
	run := func(command, marker string) string {
		c := exec.Command("sh", "-c", command)
		c.Env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "CLAUDE_PLUGIN_ROOT="+pluginRoot(t), "CLAUDE_PROJECT_DIR="+project, "VAULTMIND_HOOK_SOURCE="+marker, "VAULTMIND_VAULT=", "VAULTMIND_VAULTS=", "VAULTMIND_PROJECT_DIR="+project, "PATH="+tools+":/usr/bin:/bin")
		c.Stdin = strings.NewReader("{}")
		out, err := c.Output()
		require.NoError(t, err)
		return string(out)
	}
	out := run(command, "")
	assert.Contains(t, out, "no vault in this project")
	assert.Contains(t, out, "/vaultmind:vaultmind-setup")
	assert.Len(t, strings.Split(strings.TrimSpace(out), "\n"), 1)
	require.NoError(t, os.MkdirAll(filepath.Join(project, "vaultmind-vault"), 0755))
	assert.Contains(t, run(command, ""), "no vault in this project", "an empty conventional folder is not a vault")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "knowledge/.vaultmind"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "knowledge/.vaultmind/config.yaml"), []byte("types: {}\n"), 0644))
	out = run(command, "")
	assert.NotEmpty(t, out)
	assert.NotContains(t, out, "no vault in this project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0755))
	// The installed health hook still injects, while its plugin copy yields.
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude/settings.json"), []byte(`{"hooks":{"SessionStart":[{"hooks":[{"command":"bash .claude/scripts/vaultmind-health.sh"}]}]}}`), 0644))
	local := run(`VAULTMIND_VAULT="`+filepath.Join(project, "knowledge")+`" bash "`+filepath.Join(pluginRoot(t), "scripts/vaultmind-health.sh")+`"`, "")
	assert.NotEmpty(t, local)
	assert.Equal(t, local, run(command, "")+local)
}

func TestPluginPreCompactUsesKnowledgeProfile(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, "knowledge/.vaultmind")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("types: {}"), 0644))
	c := exec.Command("bash", filepath.Join(pluginRoot(t), "scripts/precompact-preserve.sh"))
	c.Env = append(os.Environ(), "HOME="+t.TempDir(), "CLAUDE_CONFIG_DIR=", "CLAUDE_PROJECT_DIR="+project, "VAULTMIND_PROJECT_DIR="+project, "VAULTMIND_VAULT=", "VAULTMIND_VAULTS=", "VAULTMIND_HOOK_SOURCE=plugin", "PATH=/usr/bin:/bin")
	c.Stdin = strings.NewReader(`{"session_id":"plugin-test"}`)
	out, err := c.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "LEARNED ABOUT THE CODE")
	assert.NotContains(t, string(out), "CHANGED IN YOU")
	assert.NotContains(t, string(out), "identity-who-i-am")
}

func TestPluginWrapperConcurrentFirstUse(t *testing.T) {
	url, requests := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
	data := t.TempDir()
	root := pluginRoot(t)
	trace := filepath.Join(t.TempDir(), "trace")
	env := append(os.Environ(), "PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT="+root, "CLAUDE_PLUGIN_DATA="+data, "VAULTMIND_PLUGIN_RELEASE_URL="+url, "VAULTMIND_PLUGIN_TRACE="+trace)
	// #353: this failed only in main's CI. On ANY failure, show which lock path
	// each process took — a require that stops early must not hide the trace.
	t.Cleanup(func() {
		if t.Failed() {
			traced, _ := os.ReadFile(trace)
			t.Logf("requests=%d\ntrace:\n%s", requests.Load(), traced)
		}
	})
	type result struct {
		output string
		err    error
	}
	done := make(chan result, 2)
	for range 2 {
		go func() {
			c := exec.Command("sh", filepath.Join(root, "bin/vaultmind"), "mcp")
			c.Env = env
			out, err := c.CombinedOutput()
			done <- result{string(out), err}
		}()
	}
	var outputs []string
	for range 2 {
		r := <-done
		require.NoError(t, r.err, r.output)
		assert.Contains(t, r.output, "fake:mcp")
		outputs = append(outputs, r.output)
	}
	// #353: this failed only in main's CI; the trace says which lock path each process took.
	traced, err := os.ReadFile(trace)
	require.NoError(t, err, "VAULTMIND_PLUGIN_TRACE must record the lock decisions")
	assert.Contains(t, string(traced), "lock-acquired")
	assert.EqualValues(t, 2, requests.Load(), "only archive + checksum should be fetched\noutputs:\n%s\ntrace:\n%s", strings.Join(outputs, "\n---\n"), traced)
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t)+".lock"), "the lock is released")
}

// A release PR may leave the marketplace on the immediately previous release
// until the new tag's archives and checksums exist. No older ref is allowed.
func TestPluginMarketplaceReleasePin(t *testing.T) {
	raw, err := os.ReadFile("../../.claude-plugin/marketplace.json")
	require.NoError(t, err)
	var market struct {
		Plugins []struct {
			Name   string
			Source struct{ Source, URL, Path, Ref string }
		}
	}
	require.NoError(t, json.Unmarshal(raw, &market))
	require.Len(t, market.Plugins, 1)
	source := market.Plugins[0].Source
	assert.Equal(t, "git-subdir", source.Source)
	assert.Equal(t, "peiman/vaultmind", source.URL)
	assert.Equal(t, "plugin", source.Path)
	raw, err = os.ReadFile("../../CHANGELOG.md")
	require.NoError(t, err)
	var previous string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "## [") && !strings.HasPrefix(line, "## [Unreleased]") {
			v, _, _ := strings.Cut(strings.TrimPrefix(line, "## ["), "]")
			if v != pluginVersion(t) {
				previous = "v" + v
				break
			}
		}
	}
	assert.Contains(t, []string{"v" + pluginVersion(t), previous}, source.Ref)
}

func installWrapperCopy(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude-plugin/plugin.json"), pluginRead(t, ".claude-plugin/plugin.json"), 0644))
	// Bound a regression's recursion without leaving runaway child processes.
	body := "VM_TEST_DEPTH=$((${VM_TEST_DEPTH:-0}+1)); export VM_TEST_DEPTH\n[ \"$VM_TEST_DEPTH\" -lt 4 ] || exit 91\nprintf 'called\\n' >> \"$VM_TEST_PROBES\"\n" + string(pluginRead(t, "bin/vaultmind"))
	hookscripts.InstallStub(t, filepath.Join(root, "bin"), "vaultmind", body)
}

func TestPluginWrapperSkipsOtherWrappers(t *testing.T) {
	data := t.TempDir()
	cached := filepath.Join(data, pluginVersion(t))
	require.NoError(t, os.MkdirAll(cached, 0755))
	hookscripts.InstallStub(t, cached, "vaultmind", "echo cached\n")
	other := t.TempDir()
	installWrapperCopy(t, other)
	probes := filepath.Join(t.TempDir(), "probes")
	env := []string{"PATH=" + filepath.Join(other, "bin") + ":" + filepath.Join(pluginRoot(t), "bin") + ":/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT=" + pluginRoot(t), "CLAUDE_PLUGIN_DATA=" + data, "VM_TEST_PROBES=" + probes}
	out, stderr, err := wrapperRun(t, env, "ask")
	require.NoError(t, err, stderr)
	assert.Equal(t, "cached\n", out)
	assert.NoFileExists(t, probes, "another plugin wrapper must never be probed")
	// The marker also prevents recursion through an unrecognized wrapper.
	require.NoError(t, os.Remove(filepath.Join(other, ".claude-plugin/plugin.json")))
	out, stderr, err = wrapperRun(t, append(env, "VAULTMIND_PLUGIN_PROBE=1"), "ask")
	require.NoError(t, err, stderr)
	assert.Equal(t, "cached\n", out)
	assert.NoFileExists(t, probes)
	// Without a manifest, the second wrapper is probed once and inherits the
	// exported marker. It must skip its own PATH search instead of recursing.
	out, stderr, err = wrapperRun(t, append(env, "VAULTMIND_PLUGIN_PROBE="), "ask")
	require.NoError(t, err, stderr)
	assert.Equal(t, "cached\n", out)
	raw, err := os.ReadFile(probes)
	require.NoError(t, err)
	assert.Equal(t, "called\n", string(raw))
}

func TestPluginWrapperBreaksStaleLock(t *testing.T) {
	// The lock is a file holding the owner's PID (#353). An older release's lock
	// is a directory with no readable owner, so only its age frees it.
	for _, stale := range []string{"dead PID", "old live PID", "old empty lock", "old directory lock"} {
		t.Run(stale, func(t *testing.T) {
			data := t.TempDir()
			lock := filepath.Join(data, pluginVersion(t)+".lock")
			pid := "999999999\n"
			if stale == "old live PID" {
				pid = fmt.Sprintln(os.Getpid())
			}
			switch stale {
			case "old empty lock":
				require.NoError(t, os.WriteFile(lock, nil, 0644))
			case "old directory lock":
				require.NoError(t, os.Mkdir(lock, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(lock, "pid"), []byte(pid), 0644))
			default:
				require.NoError(t, os.WriteFile(lock, []byte(pid), 0644))
			}
			if stale != "dead PID" {
				old := time.Now().Add(-10 * time.Minute)
				require.NoError(t, os.Chtimes(lock, old, old))
			}
			url, _ := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
			out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url}, "ask")
			require.NoError(t, err, stderr)
			assert.Contains(t, out, "fake:ask")
			assert.NoFileExists(t, lock)
			assert.NoDirExists(t, lock)
		})
	}
}

// #353: if another holder publishes the cache while we download, we must use its
// install, never move our payload inside it.
func TestPluginWrapperPublishLostRace(t *testing.T) {
	data := t.TempDir()
	cache := filepath.Join(data, pluginVersion(t))
	// The other holder's install, published by one rename mid-download.
	staged := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.Mkdir(staged, 0755))
	hookscripts.InstallStub(t, staged, "vaultmind", "#!/bin/sh\nprintf 'other:%s\\n' \"$*\"\n")
	var published error
	url, _ := fakeReleaseWith(t, runtime.GOOS, runtime.GOARCH, false, func(r *http.Request) {
		if strings.Contains(r.URL.Path, "checksums") {
			published = os.Rename(staged, cache)
		}
	})
	trace := filepath.Join(t.TempDir(), "trace")
	out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "VAULTMIND_PLUGIN_TRACE=" + trace}, "ask")
	require.NoError(t, published, "the simulated holder must publish before our download ends")
	require.NoError(t, err, stderr)
	assert.Contains(t, out, "other:ask")
	entries, err := os.ReadDir(cache)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, e.IsDir(), "our payload must not be nested inside the published cache: %s", e.Name())
	}
	assert.NoFileExists(t, filepath.Join(cache, "libonnxruntime.so"), "our payload must not be mixed into the published cache")
	traced, _ := os.ReadFile(trace)
	assert.Contains(t, string(traced), "publish-lost-race")
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t)+".lock"), "the lock is released")
}

// #353's root cause: Ubuntu 26.04's mkdir (uutils) lets two concurrent calls on
// one path both succeed (measured 100 of 300 races), so a mkdir lock admitted
// both first runs. This mkdir always reports success, the worst case made
// deterministic: the lock must not depend on it.
func TestPluginWrapperLockDoesNotTrustMkdir(t *testing.T) {
	url, requests := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
	data := t.TempDir()
	root := pluginRoot(t)
	tools := t.TempDir()
	hookscripts.InstallStub(t, tools, "mkdir", "#!/bin/sh\n/bin/mkdir -p \"$@\" 2>/dev/null\nexit 0\n")
	trace := filepath.Join(t.TempDir(), "trace")
	env := append(os.Environ(), "PATH="+tools+":/usr/bin:/bin", "CLAUDE_PLUGIN_ROOT="+root, "CLAUDE_PLUGIN_DATA="+data, "VAULTMIND_PLUGIN_RELEASE_URL="+url, "VAULTMIND_PLUGIN_TRACE="+trace)
	t.Cleanup(func() {
		if t.Failed() {
			traced, _ := os.ReadFile(trace)
			t.Logf("requests=%d\ntrace:\n%s", requests.Load(), traced)
		}
	})
	done := make(chan error, 2)
	for range 2 {
		go func() {
			c := exec.Command("sh", filepath.Join(root, "bin/vaultmind"), "mcp")
			c.Env = env
			out, err := c.CombinedOutput()
			if err == nil && !strings.Contains(string(out), "fake:mcp") {
				err = fmt.Errorf("no fake:mcp in %q", out)
			}
			done <- err
		}()
	}
	for range 2 {
		require.NoError(t, <-done)
	}
	assert.EqualValues(t, 2, requests.Load(), "one download, even when mkdir cannot be trusted as a lock")
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t)+".lock"))
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t)+".lock"), "the lock is released")
}

// #353 in main's CI: the other holder had claimed the cache but was still moving
// its payload in, so the binary was not there yet. The loser must wait for it
// (the binary is what every reader takes as complete), not fail.
func TestPluginWrapperLostRaceWaitsForTheWinnersBinary(t *testing.T) {
	data := t.TempDir()
	cache := filepath.Join(data, pluginVersion(t))
	staged := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.Mkdir(staged, 0755))
	hookscripts.InstallStub(t, staged, "vaultmind", "#!/bin/sh\nprintf 'other:%s\\n' \"$*\"\n")
	landed := make(chan error, 1)
	url, _ := fakeReleaseWith(t, runtime.GOOS, runtime.GOARCH, false, func(r *http.Request) {
		if !strings.Contains(r.URL.Path, "checksums") {
			return
		}
		if err := os.Mkdir(cache, 0755); err != nil { // claimed, still empty
			landed <- err
			return
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			err := os.Rename(filepath.Join(staged, "vaultmind.body"), filepath.Join(cache, "vaultmind.body"))
			if err == nil {
				err = os.Rename(filepath.Join(staged, "vaultmind"), filepath.Join(cache, "vaultmind"))
			}
			landed <- err
		}()
	})
	trace := filepath.Join(t.TempDir(), "trace")
	out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "VAULTMIND_PLUGIN_TRACE=" + trace}, "ask")
	require.NoError(t, <-landed, "the simulated winner must finish its publish")
	require.NoError(t, err, stderr)
	assert.Contains(t, out, "other:ask")
	traced, _ := os.ReadFile(trace)
	assert.Contains(t, string(traced), "publish-lost-race")
	assert.NoFileExists(t, filepath.Join(data, pluginVersion(t)+".lock"), "the lock is released")
}

// A publish killed after claiming the cache leaves it without its binary. Only the
// lock holder publishes, so the next holder starts that publish over instead of
// failing until someone deletes the folder by hand.
func TestPluginWrapperRecoversIncompleteCache(t *testing.T) {
	data := t.TempDir()
	cache := filepath.Join(data, pluginVersion(t))
	require.NoError(t, os.Mkdir(cache, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cache, "libonnxruntime.so"), []byte("partial"), 0644))
	url, _ := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
	out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url}, "ask")
	require.NoError(t, err, stderr)
	assert.Contains(t, out, "fake:ask")
	assert.FileExists(t, filepath.Join(cache, "vaultmind"))
}

// The trace is opt-in: without VAULTMIND_PLUGIN_TRACE nothing is written and stdout is the CLI's alone.
func TestPluginWrapperTraceIsOptIn(t *testing.T) {
	url, _ := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
	data := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace")
	out, stderr, err := wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "VAULTMIND_PLUGIN_TRACE="}, "ask")
	require.NoError(t, err, stderr)
	assert.Equal(t, "fake:ask\n", out)
	assert.NoFileExists(t, trace)
	out, stderr, err = wrapperRun(t, []string{"PATH=/usr/bin:/bin", "CLAUDE_PLUGIN_DATA=" + data, "VAULTMIND_PLUGIN_RELEASE_URL=" + url, "VAULTMIND_PLUGIN_TRACE=" + trace}, "ask")
	require.NoError(t, err, stderr)
	assert.Equal(t, "fake:ask\n", out)
	traced, err := os.ReadFile(trace)
	require.NoError(t, err)
	assert.Contains(t, string(traced), "cache-hit")
}

func TestPluginWrapperInfersInstalledRootAndData(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config with spaces")
	// Match the throwaway Claude install, with distinct names to test ordering
	// and the documented plugin-id sanitization (e.g. dots in marketplace names).
	root := filepath.Join(config, "plugins/cache/team.market/vaultmind", pluginVersion(t))
	installWrapperCopy(t, root)
	data := filepath.Join(config, "plugins/data/vaultmind-team-market")
	url, requests := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
	env := append(os.Environ(), "HOME="+t.TempDir(), "CLAUDE_PLUGIN_ROOT=", "CLAUDE_PLUGIN_DATA=", "PATH=/usr/bin:/bin", "VAULTMIND_PLUGIN_RELEASE_URL="+url, "VM_TEST_PROBES="+filepath.Join(t.TempDir(), "probes"))
	for _, mode := range []string{"hook", "Bash tool"} {
		c := exec.Command("sh", filepath.Join(root, "bin/vaultmind"), "ask")
		c.Env = env
		if mode == "hook" {
			c.Env = append(c.Env, "CLAUDE_PLUGIN_ROOT="+root, "CLAUDE_PLUGIN_DATA="+data)
		}
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
		assert.Contains(t, string(out), "fake:ask")
	}
	assert.EqualValues(t, 2, requests.Load(), "Bash tool must reuse the hook's binary")
	assert.FileExists(t, filepath.Join(data, pluginVersion(t), "vaultmind"))
}

func TestPluginWrapperPreservesModelCacheEnvironment(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		t.Run(fmt.Sprint(inherited), func(t *testing.T) {
			data := t.TempDir()
			cached := filepath.Join(data, pluginVersion(t))
			require.NoError(t, os.MkdirAll(cached, 0755))
			hookscripts.InstallStub(t, cached, "vaultmind", `printf '%s\n' "${XDG_CACHE_HOME-unset}"`)
			c := exec.Command("sh", filepath.Join(pluginRoot(t), "bin/vaultmind"))
			c.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "CLAUDE_PLUGIN_ROOT=" + pluginRoot(t), "CLAUDE_PLUGIN_DATA=" + data}
			want := "unset\n"
			if inherited {
				c.Env = append(c.Env, "XDG_CACHE_HOME=/shared models")
				want = "/shared models\n"
			}
			out, err := c.CombinedOutput()
			require.NoError(t, err, string(out))
			assert.Equal(t, want, string(out))
		})
	}
}

func TestPluginScriptCoexistence(t *testing.T) {
	scripts := hooks.ScriptsForProfile(hooks.ProfileKnowledge)
	require.Len(t, scripts, 6, "every knowledge-profile coexistence guard must be exercised")
	for _, name := range scripts {
		t.Run(name, func(t *testing.T) {
			project, home, tools := t.TempDir(), t.TempDir(), t.TempDir()
			vault := filepath.Join(project, "knowledge")
			require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind/config.yaml"), []byte("types: {}\n"), 0644))
			note := filepath.Join(vault, "retry-policy.md")
			require.NoError(t, os.WriteFile(note, []byte("Retry with exponential backoff"), 0644))
			hookscripts.InstallStub(t, tools, "vaultmind", `case "$1" in
 doctor) echo 'Embeddings: dense 1/1 (bge-m3), sparse 1/1, colbert 1/1';;
 tree) echo '{"result":{"vaults":[{"vault":"knowledge","root":{"notes":[{"id":"retry","title":"Retry policy"}]}}]}}';;
 *) echo '  0.42  retry  Retry with exponential backoff';;
 esac`)
			input := "{}"
			switch name {
			case "vault-recall.sh":
				input = `{"prompt":"how does the retry policy handle transient errors?"}`
			case "vault-track-read.sh":
				input = filePayload("Read", note)
			case "vault-reach.sh":
				input = filePayload("Write", note)
			case "vault-code-map.sh":
				input = filePayload("Read", filepath.Join(project, "internal/retry.go"))
			}
			run := func(marker, config string) string {
				c := exec.Command("bash", filepath.Join(".", name)) // canonical source: mutation tests must exercise it
				c.Env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR="+config, "CLAUDE_PROJECT_DIR="+project, "VAULTMIND_PROJECT_DIR="+project, "VAULTMIND_VAULT=", "VAULTMIND_VAULTS=", "VAULTMIND_HOOK_SOURCE="+marker, "PATH="+tools+":/usr/bin:/bin", "XDG_STATE_HOME="+t.TempDir(), "VAULTMIND_RECALL_VAULT=", "VAULT_PATH_PATTERN=")
				if marker == "" {
					c.Env = append(c.Env, "VAULTMIND_VAULT="+vault)
				}
				c.Stdin = strings.NewReader(input)
				out, err := c.Output()
				require.NoError(t, err)
				return string(out)
			}
			require.NotEmpty(t, run("plugin", ""), "positive control: this script must inject")
			for _, where := range []string{"project", "project local", "user", "custom config"} {
				t.Run(where, func(t *testing.T) {
					path, command, config := filepath.Join(project, ".claude/settings.json"), "bash .claude/scripts/"+name, ""
					switch where {
					case "project local":
						path = filepath.Join(project, ".claude/settings.local.json")
					case "user":
						path = filepath.Join(home, ".claude/settings.json")
						command = "bash " + home + "/.vaultmind/scripts/" + name
					case "custom config":
						config = filepath.Join(home, "custom config")
						path = filepath.Join(config, "settings.json")
						command = "bash " + home + "/.vaultmind/scripts/" + name
					}
					settings := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]string{"command": command}}}}}}
					raw, err := json.Marshal(settings)
					require.NoError(t, err)
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
					require.NoError(t, os.WriteFile(path, raw, 0644))
					assert.Empty(t, run("plugin", config), "plugin yields to installed hook")
					assert.NotEmpty(t, run("", config), "installed hook still injects")
					require.NoError(t, os.Remove(path))
				})
			}
		})
	}
}
