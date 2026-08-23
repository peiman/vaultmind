package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/doctorclient"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/meshpaths"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Mesh-doctor cmd-layer constants (SSOT). Env vars + default paths mirror the
// agent-chat MCP so doctor auto-discovers the same substrate the member runs on.
const (
	// envDaemonURL is the chat-daemon base URL env var (loopback-pinned client).
	envDaemonURL = "AGENT_CHAT_DAEMON_URL"
	// defaultDaemonURL is the agent-chat daemon's default loopback HTTP endpoint.
	defaultDaemonURL = "http://127.0.0.1:7850"
	// envAgentRegistry is the agents.yaml path env var.
	envAgentRegistry = "AGENT_CHAT_REGISTRY"
	// envProjectPath is the project-path env var used to resolve the agent slug.
	envProjectPath = "AGENT_CHAT_PROJECT_PATH"
)

// Mesh-section human labels (SSOT — no inline literals in the writer).
const (
	meshSectionHeader       = "Mesh identity (Contract-B):"
	meshKeyCustodyLabel     = "  key custody: "
	meshKeyPresentYes       = "present"
	meshKeyPresentNo        = "absent"
	meshKeyModeBad          = " (mode NOT 0600)"
	meshKeySizeBad          = " (unexpected size)"
	meshSignerLabel         = "  signer: "
	meshSignerUp            = "running"
	meshSignerDown          = "not running (OK if on-demand)"
	meshAuthLabel           = "  authentication: "
	meshDaemonLabel         = "  daemon: "
	meshDaemonReachable     = "HTTP reachable"
	meshDaemonUnreachable   = "unreachable"
	meshHeartbeatLabel      = "  watcher heartbeat: "
	meshHeartbeatFresh      = "fresh"
	meshHeartbeatAbsent     = "absent — the watcher has not run (arm it, or ignore if you are not on the mesh)"
	meshHeartbeatStale      = "STALE — the watcher may be present-but-dead"
	meshHeartbeatUnresolved = "UNRESOLVED — no agent slug; watcher liveness is UNKNOWN, not OK"
	meshWarnPrefix          = "  ⚠ "
)

// meshDoctorErrParse prefixes a --mesh-root-pubkey decode/validate failure.
const meshDoctorErrParse = "doctor: --mesh-root-pubkey is not a valid base64 ed25519 root pubkey"

// meshDoctorErrRegistry prefixes a --mesh-registry read failure.
const meshDoctorErrRegistry = "doctor: read --mesh-registry file"

// populateMeshIdentity resolves the mesh substrate (key/socket paths, the pinned
// root from --mesh-root-pubkey or the enroll-persisted anchor, the registry from
// --mesh-registry or the daemon, and the slug from --mesh-slug or agents.yaml),
// builds the keyless signer + loopback daemon client, runs the 3-tier check, and
// attaches the section to result ONLY when a mesh signal exists (nil ⇒ omitted
// from --json). It is the cmd-layer counterpart to query.BuildMeshIdentity.
func populateMeshIdentity(cmd *cobra.Command, result *query.DoctorResult) error {
	in, present, err := resolveMeshInput(cmd)
	if err != nil {
		return err
	}
	if !present {
		return nil // no mesh substrate — section omitted entirely
	}
	mi, err := query.BuildMeshIdentity(cmd.Context(), in)
	if err != nil {
		return err
	}
	if mi.HasSignal() {
		result.MeshIdentity = mi
	}
	return nil
}

// resolveMeshInput assembles the MeshDoctorInput and reports whether any mesh
// signal exists (identity key present, an anchor exists, a --mesh-* flag passed,
// or the daemon is reachable). When no signal exists, the section is skipped.
func resolveMeshInput(cmd *cobra.Command) (query.MeshDoctorInput, bool, error) {
	keyPath, _ := defaultSignerKeyPath()
	sockPath, _ := defaultSignerSocketPath()

	// Slug FIRST: the default heartbeat path is derived from it. It used to be
	// resolved thirty lines below the heartbeat block, so the default could
	// never be per-agent — one reason three watchers each invented their own
	// heartbeat filename.
	slugFlag := getConfigValueWithFlags[string](cmd, "mesh-slug", config.KeyAppDoctorMeshSlug)
	slug := resolveSlug(slugFlag)

	heartbeatPath := getConfigValueWithFlags[string](cmd, "mesh-heartbeat", config.KeyAppDoctorMeshHeartbeat)
	var lastwakePath, lastarmPath string
	if p, err := meshpaths.For(slug); err == nil {
		// One derivation, shared with `identity paths` and therefore with the
		// watcher script itself: the checker and the writer cannot disagree
		// about where the state lives, because neither derives it alone.
		if heartbeatPath == "" {
			heartbeatPath = p.Heartbeat
		}
		lastwakePath = p.Lastwake
		lastarmPath = p.Lastarm
	}
	// err ⇒ no slug ⇒ heartbeatPath stays "" unless the flag set it, and
	// checkHeartbeat reports UNRESOLVED — the truthful state — rather than a
	// filesystem verdict about a path nobody derived.

	in := query.MeshDoctorInput{
		KeyPath:       keyPath,
		SocketPath:    sockPath,
		HeartbeatPath: heartbeatPath,
		LastwakePath:  lastwakePath,
		LastarmPath:   lastarmPath,
		Now:           time.Now(),
		Signer:        &signer.Client{SocketPath: sockPath},
	}

	rootFlag := getConfigValueWithFlags[string](cmd, "mesh-root-pubkey", config.KeyAppDoctorMeshRootPubkey)
	registryFlag := getConfigValueWithFlags[string](cmd, "mesh-registry", config.KeyAppDoctorMeshRegistry)

	anchorPresent, err := applyPinAndNetwork(&in, rootFlag)
	if err != nil {
		return query.MeshDoctorInput{}, false, err
	}

	registryPresent, err := applyOfflineRegistry(&in, registryFlag)
	if err != nil {
		return query.MeshDoctorInput{}, false, err
	}

	in.Slug = slug

	daemon := newDoctorDaemonClient()
	in.Daemon = daemon
	daemonReachable := daemon != nil && daemonIsReachable(cmd.Context(), daemon)

	keyPresent := keyFileExists(keyPath)
	flagPassed := rootFlag != "" || registryFlag != "" || slugFlag != "" ||
		cmd.Flags().Changed("mesh-heartbeat")
	present := keyPresent || anchorPresent || registryPresent || flagPassed || daemonReachable
	return in, present, nil
}

// applyPinAndNetwork sets the pinned root + network id from --mesh-root-pubkey
// when given, else auto-discovers the FIRST persisted anchor. It returns whether
// an anchor was found (a mesh signal). An explicit --mesh-root-pubkey that does
// not decode is a hard error (the operator asked for a specific pin).
func applyPinAndNetwork(in *query.MeshDoctorInput, rootFlag string) (bool, error) {
	if rootFlag != "" {
		raw, err := base64.StdEncoding.DecodeString(rootFlag)
		if err != nil {
			return false, fmt.Errorf("%s: %w", meshDoctorErrParse, err)
		}
		pk, err := registry.NewPublicKey(raw)
		if err != nil {
			return false, fmt.Errorf("%s: %w", meshDoctorErrParse, err)
		}
		in.PinnedRootPub = pk.Bytes()
		in.NetworkID = registry.NetworkID(pk.Bytes())
		return false, nil
	}

	anchorPath, err := defaultNetworkAnchorPath()
	if err != nil {
		return false, nil //nolint:nilerr // anchor path unresolved ⇒ no pin, not fatal
	}
	anchors, err := anchor.Load(anchorPath)
	if err != nil || len(anchors) == 0 {
		return false, nil //nolint:nilerr // missing/corrupt anchor ⇒ unpinned path
	}
	a := anchors[0]
	raw, err := base64.StdEncoding.DecodeString(a.RootPubKey)
	if err != nil {
		return true, nil //nolint:nilerr // present-but-undecodable ⇒ stay unpinned
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return true, nil //nolint:nilerr // present-but-invalid ⇒ stay unpinned
	}
	in.PinnedRootPub = pk.Bytes()
	in.NetworkID = a.NetworkID
	return true, nil
}

// applyOfflineRegistry reads a --mesh-registry file into the input when given.
// A passed-but-unreadable file is a hard error (the operator named a file).
func applyOfflineRegistry(in *query.MeshDoctorInput, registryFlag string) (bool, error) {
	if registryFlag == "" {
		return false, nil
	}
	// registryFlag is an operator-supplied path (explicit --mesh-registry flag),
	// not attacker-controlled input — same trust class as the vault path.
	// #nosec G304
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(registryFlag)
	if err != nil {
		return false, fmt.Errorf("%s: %w", meshDoctorErrRegistry, err)
	}
	in.RegistryBytes = raw
	return true, nil
}

// resolveSlug returns the explicit --mesh-slug when given, else resolves it from
// the agents.yaml registry by matching the project path. An unresolvable slug is
// not an error — tier-2 then reports not-enrolled.
func resolveSlug(slugFlag string) string {
	if slugFlag != "" {
		// Contract-B registry bindings store the BARE slug; "agent:" is only the
		// chat wire principal. Strip it so an explicit --mesh-slug resolves.
		return strings.TrimPrefix(slugFlag, "agent:")
	}
	return slugFromAgentsYAML(os.Getenv(envAgentRegistry), projectPath())
}

// projectPath returns AGENT_CHAT_PROJECT_PATH, falling back to the working dir.
func projectPath() string {
	if p := os.Getenv(envProjectPath); p != "" {
		return p
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// agentsYAML is the minimal shape of the chat-mcp agents.yaml the slug resolver
// reads: a list of {slug, project_path}. Other fields are ignored.
type agentsYAML struct {
	Agents []struct {
		Slug        string `yaml:"slug"`
		ProjectPath string `yaml:"project_path"`
	} `yaml:"agents"`
}

// slugFromAgentsYAML resolves the agent slug whose project_path matches
// projectDir (normalized via filepath.Clean). Returns "" when the registry path
// is empty/unreadable or no entry matches. The slug is a LABEL — authenticity is
// the tier-2 selfVerify, not the slug — so a wrong/missing slug is non-fatal.
func slugFromAgentsYAML(registryPath, projectDir string) string {
	if registryPath == "" || projectDir == "" {
		return ""
	}
	// registryPath is an operator-controlled env var (AGENT_CHAT_REGISTRY), the
	// same path the chat MCP itself reads — not attacker-controlled input.
	// The newer gosec taint pass (G703) flags the same env-var→ReadFile flow;
	// the operator-controlled justification above covers it equally.
	// #nosec G304 G703
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		return ""
	}
	var ay agentsYAML
	if err := yaml.Unmarshal(raw, &ay); err != nil {
		return ""
	}
	want := filepath.Clean(projectDir)
	for _, a := range ay.Agents {
		if filepath.Clean(a.ProjectPath) == want {
			// Return the BARE slug: Contract-B registry bindings key on the bare
			// slug ("agent:" is only the chat wire principal). Prefixing it here
			// made registry.Resolve miss an enrolled member (false not-enrolled).
			return strings.TrimPrefix(a.Slug, "agent:")
		}
	}
	return ""
}

// newDoctorDaemonClient builds the loopback-pinned read-only daemon client from
// the daemon URL env (default loopback). A construction failure (bad URL) yields
// nil — tier-3 then reports the daemon unreachable rather than erroring doctor.
func newDoctorDaemonClient() query.MeshDaemonClient {
	url := os.Getenv(envDaemonURL)
	if url == "" {
		url = defaultDaemonURL
	}
	c, err := doctorclient.New(url)
	if err != nil {
		return nil
	}
	return c
}

// daemonIsReachable probes whoami to decide presence-gating without surfacing an
// error (an unreachable daemon is simply "no daemon signal").
func daemonIsReachable(ctx context.Context, d query.MeshDaemonClient) bool {
	reachable, _, _ := d.Whoami(ctx)
	return reachable
}

// keyFileExists reports whether the identity key file exists (Lstat — never
// reads it). Used only for presence-gating the section.
func keyFileExists(keyPath string) bool {
	if keyPath == "" {
		return false
	}
	_, err := os.Lstat(keyPath)
	return err == nil
}

// writeMeshIdentity renders the mesh-identity section to w. It is conditionally
// present (nil ⇒ nothing printed), mirroring writeEmbeddingStatus's gate. Every
// section warning is printed; the authenticated verdict reuses the section
// Status so human + --json never disagree.
func writeMeshIdentity(w io.Writer, mi *query.DoctorMeshIdentity, _ bool) error {
	if mi == nil {
		return nil
	}
	if _, err := fmt.Fprintln(w, meshSectionHeader); err != nil {
		return err
	}
	if err := writeMeshCustody(w, mi); err != nil {
		return err
	}
	if err := writeMeshAuth(w, mi); err != nil {
		return err
	}
	if err := writeMeshDaemon(w, mi); err != nil {
		return err
	}
	return writeMeshWarnings(w, mi)
}

// writeMeshCustody prints the tier-1 custody + signer lines.
func writeMeshCustody(w io.Writer, mi *query.DoctorMeshIdentity) error {
	custody := meshKeyPresentNo
	if mi.KeyPresent {
		custody = meshKeyPresentYes
		if !mi.KeyModeOK {
			custody += meshKeyModeBad
		}
		if !mi.KeySizeOK {
			custody += meshKeySizeBad
		}
	}
	if _, err := fmt.Fprintln(w, meshKeyCustodyLabel+custody); err != nil {
		return err
	}
	signerState := meshSignerDown
	if mi.SignerReachable {
		signerState = meshSignerUp
	}
	_, err := fmt.Fprintln(w, meshSignerLabel+signerState)
	return err
}

// writeMeshAuth prints the tier-2 authentication verdict (the section Status,
// reserving "authenticated" for the cryptographically-proven green state).
func writeMeshAuth(w io.Writer, mi *query.DoctorMeshIdentity) error {
	_, err := fmt.Fprintln(w, meshAuthLabel+mi.Status)
	return err
}

// writeMeshDaemon prints the tier-3 daemon reachability + heartbeat lines.
func writeMeshDaemon(w io.Writer, mi *query.DoctorMeshIdentity) error {
	daemon := meshDaemonUnreachable
	if mi.DaemonReachable {
		daemon = meshDaemonReachable
		if mi.DaemonMode != "" {
			daemon += " (" + mi.DaemonMode + ")"
		}
	}
	if _, err := fmt.Fprintln(w, meshDaemonLabel+daemon); err != nil {
		return err
	}
	// One line, ALWAYS, and it always names the resolved path. The old switch
	// printed nothing at all on stale, and no state ever disclosed WHERE the
	// check had looked — which is precisely how a checker reading a directory
	// nothing writes to stayed invisible for months. A verdict about a file
	// whose location is undisclosed cannot be audited by anyone.
	var line string
	switch mi.WatcherHeartbeatState {
	case query.HeartbeatFresh:
		line = fmt.Sprintf("%s%s (%ds) — %s", meshHeartbeatLabel, meshHeartbeatFresh,
			mi.WatcherHeartbeatAge, mi.WatcherHeartbeatPath)
	case query.HeartbeatStale:
		line = fmt.Sprintf("%s%s (%ds) — %s", meshHeartbeatLabel, meshHeartbeatStale,
			mi.WatcherHeartbeatAge, mi.WatcherHeartbeatPath)
	case query.HeartbeatAbsent:
		line = fmt.Sprintf("%s%s — %s", meshHeartbeatLabel, meshHeartbeatAbsent,
			mi.WatcherHeartbeatPath)
	default: // unresolved, and any state this renderer does not know
		line = meshHeartbeatLabel + meshHeartbeatUnresolved
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}
	return nil
}

// writeMeshWarnings prints each section warning under the section.
func writeMeshWarnings(w io.Writer, mi *query.DoctorMeshIdentity) error {
	for _, warn := range mi.Warnings {
		if _, err := fmt.Fprintln(w, meshWarnPrefix+warn); err != nil {
			return err
		}
	}
	return nil
}
