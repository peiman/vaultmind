package hookscripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMeshWatch_CarriesNoIdentityLiterals pins the property that makes this
// copy canonical: ONE script serves every agent, because identity arrives via
// `eval "$(vaultmind identity paths)"` and nothing is baked in.
//
// Its three ancestors diverged precisely here — each carried its own slug (one
// spelled it four times, because a quoted heredoc cannot interpolate), its own
// daemon URL, and its own state paths, aligned only by comments. Two of those
// comments were false on darwin for months.
func TestMeshWatch_CarriesNoIdentityLiterals(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok, "mesh-watch.sh must be embedded")
	s := string(body)

	require.NotContains(t, s, "agent:mira")
	require.NotContains(t, s, "agent:workhorse")
	require.NotContains(t, s, "human:peiman", "the operator is data from identity paths, not a literal")
	require.NotContains(t, s, "human:siavoush", "an ancestor hardcoded a third party's principal into its wake filter")
	require.NotContains(t, s, "100.64.", "no baked daemon address")
	require.NotContains(t, s, "127.0.0.1", "not even the loopback default — the URL comes from identity paths")
	require.NotContains(t, s, ".config/vaultmind", "no derived state path — VM_MESH_* only")
	require.NotContains(t, s, "mesh-watch-wh", "no ancestor's per-agent filename")

	require.Contains(t, s, `grep '^VM_MESH_'`, "identity must come from the binary, filtered before eval")
	require.Contains(t, s, "refusing to arm", "no identity ⇒ no arm, loudly")
}

// runDetector extracts the DETECT heredoc from the embedded script and runs it
// as the script would — through python3, identity via env. Testing the
// extracted program (not a copy pasted into the test) means the test cannot
// drift from the shipped detector.
func runDetector(t *testing.T, env map[string]string, stdin string) (string, int) {
	t.Helper()
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)
	start := strings.Index(s, "read -r -d '' DETECT <<'PY' || true\n")
	require.Positive(t, start, "DETECT heredoc must exist")
	start += len("read -r -d '' DETECT <<'PY' || true\n")
	end := strings.Index(s[start:], "\nPY\n")
	require.Positive(t, end, "DETECT heredoc must be terminated — a truncated heredoc is an empty detector that never wakes")
	program := s[start : start+end]

	cmd := exec.Command("python3", "-c", program)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, okE := err.(*exec.ExitError); okE {
			return strings.TrimSpace(string(out)), ee.ExitCode()
		}
		t.Fatalf("running detector: %v", err)
	}
	return strings.TrimSpace(string(out)), 0
}

func detectorEnv(t *testing.T, overrides map[string]string) map[string]string {
	env := map[string]string{
		"VM_WAKE_SELF":        "agent:me",
		"VM_WAKE_OPERATOR":    "human:op",
		"VM_WAKE_SCOPE":       "all",
		"VM_WAKE_ROOM_SINCE":  "0",
		"VM_WAKE_DM_SINCE":    "0",
		"VM_WAKE_LISTEN_FILE": filepath.Join(t.TempDir(), "absent.json"),
		"VM_WAKE_FROM":        "",
		"VM_WAKE_KEYWORDS":    "",
	}
	for k, v := range overrides {
		env[k] = v
	}
	return env
}

// TestMeshWatchDetector_Behaviour drives the SHIPPED detector through the cases
// that were each, at some point, a live failure in an ancestor.
func TestMeshWatchDetector_Behaviour(t *testing.T) {
	t.Run("wakes on another agent's room message", func(t *testing.T) {
		out, rc := runDetector(t, detectorEnv(t, nil),
			`{"messages":[{"from_agent":"agent:other","ts":100,"room":"mesh"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "agent:other|100", out)
	})

	t.Run("never wakes on self", func(t *testing.T) {
		out, rc := runDetector(t, detectorEnv(t, nil),
			`{"messages":[{"from_agent":"agent:me","ts":100,"room":"mesh"}]}`)
		require.Zero(t, rc)
		require.Empty(t, out)
	})

	t.Run("room floor filters replays; DM floor is independent", func(t *testing.T) {
		// Wire since had to undershoot for the DM stream: a room message at
		// ts=50 is a replay (room floor 90) but a DM at ts=60 is genuinely new
		// (DM floor 40). The DM must wake; the room replay must not.
		env := detectorEnv(t, map[string]string{"VM_WAKE_ROOM_SINCE": "90", "VM_WAKE_DM_SINCE": "40"})
		out, rc := runDetector(t, env,
			`{"messages":[{"from_agent":"agent:other","ts":50,"room":"mesh"},{"from_agent":"agent:other","ts":60,"to_agent":"agent:me"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "agent:other|60", out)
	})

	t.Run("filtered scope: keyword wakes, chatter does not", func(t *testing.T) {
		env := detectorEnv(t, map[string]string{"VM_WAKE_SCOPE": "filtered", "VM_WAKE_KEYWORDS": "me"})
		out, rc := runDetector(t, env,
			`{"messages":[{"from_agent":"agent:other","ts":10,"room":"mesh","body":"cross-agent chatter"}]}`)
		require.Zero(t, rc)
		require.Empty(t, out, "unrelated chatter must not wake a filtered stream")

		out, rc = runDetector(t, env,
			`{"messages":[{"from_agent":"agent:other","ts":11,"room":"mesh","body":"hey ME, look at this"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "agent:other|11", out, "keyword match is case-insensitive")
	})

	t.Run("filtered scope: wake_from principal always wakes", func(t *testing.T) {
		env := detectorEnv(t, map[string]string{"VM_WAKE_SCOPE": "filtered", "VM_WAKE_FROM": "human:op,human:other"})
		out, rc := runDetector(t, env,
			`{"messages":[{"from_agent":"human:other","ts":12,"room":"mesh","body":"no keyword here"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "human:other|12", out,
			"wake_from is DATA — an ancestor hardcoded human:siavoush for exactly this case")
	})

	t.Run("DMs are always relevant regardless of scope", func(t *testing.T) {
		env := detectorEnv(t, map[string]string{"VM_WAKE_SCOPE": "filtered", "VM_WAKE_KEYWORDS": "nomatch"})
		out, rc := runDetector(t, env,
			`{"messages":[{"from_agent":"agent:other","ts":13,"to_agent":"agent:me","body":"direct"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "agent:other|13", out,
			"a message addressed to me must never be scope-filtered out")
	})

	t.Run("listen mute is honored, but the operator cannot be muted", func(t *testing.T) {
		listen := filepath.Join(t.TempDir(), "listen.json")
		require.NoError(t, os.WriteFile(listen,
			[]byte(`{"mode":"except","mute":["agent:noisy","human:op"],"hard":false}`), 0o600))
		env := detectorEnv(t, map[string]string{"VM_WAKE_LISTEN_FILE": listen})

		out, rc := runDetector(t, env,
			`{"messages":[{"from_agent":"agent:noisy","ts":14,"room":"mesh"}]}`)
		require.Zero(t, rc)
		require.Empty(t, out, "muted agent stays muted")

		out, rc = runDetector(t, env,
			`{"messages":[{"from_agent":"human:op","ts":15,"room":"mesh"}]}`)
		require.Zero(t, rc)
		require.Equal(t, "human:op|15", out,
			"without hard mode, the operator is heard even when listed in mute")
	})

	t.Run("garbage input exits nonzero — never reads as no-message", func(t *testing.T) {
		_, rc := runDetector(t, detectorEnv(t, nil), `this is not json`)
		require.NotZero(t, rc, "a parse failure must escalate; an ancestor's guard was one integer wide")
	})
}

// TestMeshWatch_EvalsOnlyVMLines pins the fix for the injection surface
// workhorse hit on their first arm of the canonical script: `identity paths`
// stdout carried a JSON log line (their console-log config routed info to
// stdout), eval executed it, and bash reported `level:info: command not found`.
// Harmless that time; eval of unfiltered output means any stdout line with
// shell metacharacters runs as code in the watcher's context.
//
// The contract: the script evals ONLY lines matching ^VM_MESH_. A binary that
// keeps its stdout clean is good hygiene; a script that does not trust it is
// the actual guard.
func TestMeshWatch_EvalsOnlyVMLines(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)

	require.NotContains(t, s, `eval "$PATHS_OUT"`,
		"raw eval of the bootstrap output is the injection surface")
	require.Contains(t, s, `grep '^VM_MESH_'`,
		"only the VM_MESH_ assignment lines may reach eval")

	// Behavioural half: run the actual bootstrap fragment with a poisoned
	// identity-paths stand-in and prove the poison line never executes.
	dir := t.TempDir()
	marker := filepath.Join(dir, "injected")
	fake := filepath.Join(dir, "vaultmind")
	require.NoError(t, os.WriteFile(fake, []byte(
		"#!/bin/bash\n"+
			"echo '{\"level\":\"info\",\"message\":\"nag\"}'\n"+ // the log-line case
			"echo 'touch "+marker+"'\n"+ // the actual injection case
			"echo \"VM_MESH_SLUG='mira'\"\n"), 0o755))

	start := strings.Index(s, `PATHS_OUT=`)
	require.Positive(t, start)
	end := strings.Index(s[start:], "\n\n")
	require.Positive(t, end)
	bootstrap := s[start : start+end]

	out, err := exec.Command("bash", "-c",
		"PATH="+dir+":$PATH\n"+bootstrap+"\nprintf '%s' \"$VM_MESH_SLUG\"").CombinedOutput()
	require.NoError(t, err, "bootstrap must succeed on poisoned-but-parseable output: %s", out)
	require.Equal(t, "mira", string(out), "the VM_ line must still be applied")
	require.NoFileExists(t, marker, "the injected command must never execute")
}

// The registry countdown must ride the WAKE line. A freshness warning that
// lives only in `doctor` is a warning nobody reads: on 2026-09-06 the registry
// aged past the hub's bound and every signed send was refused for ~21 hours
// while reads, the watcher and every check stayed green. The wake line is the
// one message an agent provably reads at the moment it is about to use chat.

func TestMeshWatch_WakeLineCarriesTheRegistryCountdown(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)
	require.Contains(t, s, "VM_MESH_REGISTRY_DAYS_LEFT",
		"the wake path must surface the countdown the emitter now provides")
}

func TestMeshWatch_CountdownIsOmittedWhenUnknown(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)
	// Guarded, not interpolated blindly: with no declared bound the variable is
	// empty and the wake line must not grow a dangling "registry fresh for  days".
	require.Contains(t, s, `[ -n "${VM_MESH_REGISTRY_DAYS_LEFT:-}" ]`,
		"an unknown countdown must be omitted, never rendered blank")
}

// runCountdown extracts registry_countdown() from the embedded script and runs
// it through bash with a given VM_MESH_REGISTRY_DAYS_LEFT. Behaviour, not text:
// a string assertion would pass a function that never fires.
func runCountdown(t *testing.T, daysLeft string) string {
	t.Helper()
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)

	start := strings.Index(s, "registry_countdown() {")
	require.Positive(t, start, "registry_countdown must exist to be tested")
	end := strings.Index(s[start:], "\n}\n")
	require.Positive(t, end, "unterminated function")
	fn := s[start : start+end+3]

	return runCountdownFn(t, fn, daysLeft, "echo \"VM_MESH_REGISTRY_DAYS_LEFT='"+daysLeft+"'\"")
}

// runCountdownFn runs the countdown with the ARM-TIME value in the environment
// and a fake `vaultmind` on PATH whose `identity paths` body is fakeBody — the
// value as of NOW. The countdown re-reads at wake, so the fake decides.
func runCountdownFn(t *testing.T, fn, armValue, fakeBody string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vaultmind"),
		[]byte("#!/bin/sh\n"+fakeBody+"\n"), 0o755)) //nolint:gosec // G306: test fixture must be executable

	script := "set -u\n" + fn + "\nregistry_countdown\n"
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "VM_MESH_REGISTRY_DAYS_LEFT="+armValue, "PATH="+dir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "countdown must never fail the wake line: %s", out)
	return string(out)
}

func countdownFn(t *testing.T) string {
	t.Helper()
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	s := string(body)
	start := strings.Index(s, "registry_countdown() {")
	require.Positive(t, start)
	end := strings.Index(s[start:], "\n}\n")
	require.Positive(t, end)
	return s[start : start+end+3]
}

// LIVE-OBSERVED (2026-09-23): a quiet-heartbeat line said "registry fresh for
// 12 more day(s)" while the hub had been serving a fresh 30-day registry for
// hours. The watcher computed the countdown when ARMED and printed it at WAKE,
// five hours later, across a re-sign. The number is read again at wake.
func TestMeshWatchCountdown_ReadsTheValueAtWakeNotAtArm(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "12", "echo \"VM_MESH_REGISTRY_DAYS_LEFT='29'\"")
	require.Contains(t, out, "29 more day(s)", "the wake line must carry the value as of now")
	require.NotContains(t, out, "12", "the arm-time value must not survive a successful re-read")
}

// If the re-read fails, the arm-time number is still the best information —
// but it must SAY it is old. An unlabeled stale number is the defect itself.
func TestMeshWatchCountdown_AFailedReReadLabelsTheOldValue(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "12", "exit 1")
	require.Contains(t, out, "12 more day(s)")
	require.Contains(t, out, "as of arm time")
}

// A bound that is no longer declared at wake yields no countdown, not the old one.
func TestMeshWatchCountdown_AnUndeclaredBoundAtWakeEmitsNothing(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "12", "echo \"VM_MESH_SLUG='mira'\"")
	require.Empty(t, out)
}

func TestMeshWatchCountdown_HealthyIsQuietButInformative(t *testing.T) {
	out := runCountdown(t, "23")
	require.Contains(t, out, "23 more day(s)")
	require.NotContains(t, out, "EXPIRES")
}

func TestMeshWatchCountdown_WarnsInsideTheLastWeek(t *testing.T) {
	require.Contains(t, runCountdown(t, "5"), "EXPIRES IN 5 DAY(S)")
}

func TestMeshWatchCountdown_PastDueNamesTheConsequence(t *testing.T) {
	out := runCountdown(t, "-2")
	require.Contains(t, out, "REGISTRY STALE")
	require.Contains(t, out, "SENDS", "the symptom is invisible from inside; the line must say sends are refused")
}

func TestMeshWatchCountdown_UnknownEmitsNothing(t *testing.T) {
	require.Empty(t, runCountdown(t, ""),
		"no declared bound ⇒ no countdown, and no dangling text in the wake line")
}

// REVIEW FINDING (2026-09-06): the previous version of this test asserted only
// require.NotPanics — vacuous in Go, since the helper already require.NoError's
// and shelling out cannot panic. It would have passed against ANY output,
// including the one the code actually produced: "[registry fresh for garbage
// more day(s)]". An unparseable value fell into the REASSURING branch, which is
// calling corruption healthy — the failure class this whole feature treats.
func TestMeshWatchCountdown_MalformedValueIsLoudNotReassuring(t *testing.T) {
	for _, bad := range []string{"garbage", "1e3", "  ", "12x"} {
		out := runCountdown(t, bad)
		require.NotContains(t, out, "fresh for",
			"a value that is not a number must never render as freshness (input %q)", bad)
		require.Contains(t, out, "UNAVAILABLE",
			"unknown must read as unknown (input %q)", bad)
	}
}

// Days-left floors, so 0 means "up to 24 hours of send-life LEFT" — still
// working. Claiming sends are refused on that day is a false present-tense
// alarm whose remedy is an offline-root ceremony; the suite previously jumped
// from 5 straight to -2 and never covered the boundary.
func TestMeshWatchCountdown_ZeroIsTheLastGoodDayNotPastDue(t *testing.T) {
	out := runCountdown(t, "0")
	require.NotContains(t, out, "REFUSED", "0 days left is still valid; sends are not being refused yet")
	require.Contains(t, out, "EXPIRES")
}

func TestMeshWatchCountdown_PastDueReadsWithoutADoubleNegative(t *testing.T) {
	out := runCountdown(t, "-2")
	require.Contains(t, out, "REGISTRY STALE")
	require.NotContains(t, out, "-2 day(s) past", "the sign is already carried by the word past")
	require.Contains(t, out, "2 day(s) past")
}

// VERIFICATION-REVIEW FINDING: the numeric guard admitted any value with an
// interior or trailing '-', so "5-3" and a bignum "-999999999999999999999"
// both fell through to the REASSURING branch — the last one a past-due value
// rendered as fresh, verbatim the failure the guard was added to stop.
func TestMeshWatchCountdown_RejectsMalformedNumericShapes(t *testing.T) {
	for _, bad := range []string{"5-", "5-3", "0-", "-999999999999999999999", "--3", "-"} {
		out := runCountdown(t, bad)
		require.NotContains(t, out, "fresh for",
			"a value bash cannot compare must never render as freshness (input %q)", bad)
	}
}

// Member machines had no local copy of the registry (dabir, 2026-09-22), so
// there was nothing to count down from. The countdown now FETCHES first — the
// hub's copy, verified by `identity fetch-registry` — then reads the number, so
// a re-sign reaches every watcher's next wake without anyone copying a file.
func TestMeshWatchCountdown_FetchesTheRegistryBeforeCounting(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	out := runCountdownFn(t, countdownFn(t), "3",
		`echo "$*" >> '`+calls+`'`+"\n"+
			`[ "$2" = paths ] && echo "VM_MESH_REGISTRY_DAYS_LEFT='29'"`+"\nexit 0")
	require.Contains(t, out, "29 more day(s)")
	got, err := os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, "identity fetch-registry\nidentity paths\n", string(got),
		"fetch first, then count: the other order prints the pre-fetch number")
}

// A fetch that fails is SAID, in the line the agent reads — a silent failure
// here is how a machine keeps counting down from a registry the hub replaced.
// It never blocks the wake.
func TestMeshWatchCountdown_AFailedFetchIsSaidNotSwallowed(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "29",
		`if [ "$2" = fetch-registry ]; then echo '{"level":"info"}' >&2; echo 'Error: no pinned root key: pass --root-pubkey' >&2; exit 1; fi`+"\n"+
			`echo "VM_MESH_REGISTRY_DAYS_LEFT='29'"`)
	require.Contains(t, out, "registry fetch failed: no pinned root key: pass --root-pubkey")
	require.NotContains(t, out, "level", "the error line, not the log noise")
	require.Contains(t, out, "29 more day(s)", "the countdown still runs from the local copy")
}

// With no local registry there is no countdown — and that is exactly the
// machine where the fetch failing matters most.
func TestMeshWatchCountdown_AFailedFetchIsSaidEvenWithNoCountdown(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "",
		`if [ "$2" = fetch-registry ]; then echo 'Error: no hub address' >&2; exit 1; fi`+"\n"+
			`echo "VM_MESH_REGISTRY_DAYS_LEFT=''"`)
	require.Contains(t, out, "registry fetch failed: no hub address")
}

func TestMeshWatchCountdown_AFailedFetchWithNoOutputStillSaysSo(t *testing.T) {
	out := runCountdownFn(t, countdownFn(t), "",
		`[ "$2" = fetch-registry ] && exit 1`+"\n"+`echo "VM_MESH_REGISTRY_DAYS_LEFT=''"`)
	require.Contains(t, out, "registry fetch failed: (no output)")
}

// `identity paths` falls back to the CURRENT DIRECTORY for the project, so a
// watcher started while the shell sat in a subfolder resolved no identity and
// refused to arm — seen 2026-09-24 when a session had cd'd into an experiment
// folder. The script pins the project it belongs to before asking.
func TestMeshWatch_AsksAboutItsProjectNotTheCurrentFolder(t *testing.T) {
	body, ok := Get("mesh-watch.sh")
	require.True(t, ok)
	project := t.TempDir()
	scripts := filepath.Join(project, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	script := filepath.Join(scripts, "mesh-watch.sh")
	require.NoError(t, os.WriteFile(script, body, 0o700)) //nolint:gosec // G306: must be executable

	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "seen")
	fake := "#!/bin/sh\nprintf '%s' \"$AGENT_CHAT_PROJECT_PATH\" > '" + seen + "'\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "vaultmind"), []byte(fake), 0o755)) //nolint:gosec // G306: test fixture must be executable

	elsewhere := t.TempDir()
	cmd := exec.Command("bash", script)
	cmd.Dir = elsewhere
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "CLAUDE_PROJECT_DIR=" + project}
	_ = cmd.Run() // refuses to arm: the fake resolves nothing

	got, err := os.ReadFile(seen) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	assert.Equal(t, project, string(got), "asked about the project, not %s", elsewhere)
}
