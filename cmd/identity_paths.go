package cmd

import (
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/meshpaths"
	"github.com/spf13/cobra"
)

var identityPathsCmd = MustNewCommand(commands.IdentityPathsMetadata, runIdentityPaths)

func init() {
	identityCmd.AddCommand(identityPathsCmd)
	setupCommandConfig(identityPathsCmd)
}

// meshIdentity is the resolved answer: who this agent is on the mesh and where
// its watcher state lives. One derivation, two renderers.
type meshIdentity struct {
	Slug       string `json:"slug"`
	Self       string `json:"self"`
	SlugSource string `json:"slug_source"`
	Dir        string `json:"dir"`
	Heartbeat  string `json:"heartbeat"`
	Pid        string `json:"pid"`
	Lastwake   string `json:"lastwake"`
	Lastarm    string `json:"lastarm"`
	Disarm     string `json:"disarm"`
	Log        string `json:"log"`
	Listen     string `json:"listen"`
	Registry   string `json:"registry"`
	DaemonURL  string `json:"daemon_url"`
}

// resolveMeshIdentity is the single derivation. The slug resolver is doctor's —
// the same agents.yaml match chat-mcp performs — so the watcher, its checker,
// and the chat connector cannot each hold a different opinion of who this is.
func resolveMeshIdentity() (meshIdentity, error) {
	slug := resolveSlug("")
	if slug == "" {
		return meshIdentity{}, fmt.Errorf(
			"identity paths: no agent slug resolvable — checked registry %q against project %q; "+
				"a watcher must not arm without an identity (set AGENT_CHAT_REGISTRY / AGENT_CHAT_PROJECT_PATH, "+
				"or add this project to agents.yaml)",
			registryPath(), projectPath())
	}
	p, err := meshpaths.For(slug)
	if err != nil {
		return meshIdentity{}, fmt.Errorf("identity paths: %w", err)
	}
	// The daemon address is identity data, resolved like the slug: explicit env
	// override, else the registry (per-agent daemon_url, else top-level), else
	// REFUSE. There is deliberately no default: a fossil pre-migration daemon on
	// this machine answers the old loopback port with weeks-stale data, and a
	// watcher armed against it would heartbeat fresh while deaf to the real
	// mesh — provably alive by every check, watching nothing. A default port is
	// where a fossil lives. (workhorse, first canonical adoption, 2026-08-23.)
	daemon := os.Getenv(envDaemonURL)
	if daemon == "" {
		daemon = daemonFromAgentsYAML(registryPath(), projectPath())
	}
	if daemon == "" {
		return meshIdentity{}, fmt.Errorf(
			"identity paths: no chat-daemon address — the registry %q carries no daemon_url for this "+
				"agent and AGENT_CHAT_DAEMON_URL is unset; a watcher must not arm against a guessed "+
				"port (add daemon_url to agents.yaml)",
			registryPath())
	}
	src := registryPath()
	if os.Getenv(envAgentRegistry) == "" {
		src += " (default; AGENT_CHAT_REGISTRY unset)"
	}
	return meshIdentity{
		Slug:       slug,
		Self:       "agent:" + slug,
		SlugSource: src,
		Dir:        p.Dir,
		Heartbeat:  p.Heartbeat,
		Pid:        p.Pid,
		Lastwake:   p.Lastwake,
		Lastarm:    p.Lastarm,
		Disarm:     p.Disarm,
		Log:        p.Log,
		Listen:     p.Listen,
		Registry:   p.Registry,
		DaemonURL:  daemon,
	}, nil
}

func runIdentityPaths(cmd *cobra.Command, _ []string) error {
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppIdentitypathsJson)

	id, err := resolveMeshIdentity()
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "identity paths", "no_identity", err.Error())
		}
		return err
	}

	if jsonOut {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), "identity paths", id, "", "")
	}

	// Shell form. Every value single-quoted: paths may carry spaces (the mesh
	// dir historically lived under "Application Support") and quoting only the
	// ones that look like they need it is how an eval breaks two months later.
	for _, kv := range [][2]string{
		{"VM_MESH_SLUG", id.Slug},
		{"VM_MESH_SELF", id.Self},
		{"VM_MESH_SLUG_SOURCE", id.SlugSource},
		{"VM_MESH_DIR", id.Dir},
		{"VM_MESH_HEARTBEAT", id.Heartbeat},
		{"VM_MESH_PID", id.Pid},
		{"VM_MESH_LASTWAKE", id.Lastwake},
		{"VM_MESH_LASTARM", id.Lastarm},
		{"VM_MESH_DISARM", id.Disarm},
		{"VM_MESH_LOG", id.Log},
		{"VM_MESH_LISTEN", id.Listen},
		{"VM_MESH_REGISTRY", id.Registry},
		{"VM_MESH_DAEMON", id.DaemonURL},
	} {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", kv[0], shellSingleQuote(kv[1])); err != nil {
			return err
		}
	}
	return nil
}

// shellSingleQuote wraps s for a shell eval, escaping embedded single quotes
// with the standard '\” idiom. Duplicated from internal/hooks.singleQuote
// deliberately NOT — reuse it if it moves somewhere importable; for now cmd
// cannot import it (unexported, other package), so this is the one copy in cmd.
func shellSingleQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}
