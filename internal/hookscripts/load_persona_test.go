package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// load-persona.sh had NO test that executed it.
//
// A census of entry points found 36 test mentions of this script and not one
// that ran it — all were strings.Contains assertions on the embedded bytes or
// settings-wiring checks. It is the SessionStart hook carrying the product's
// headline promise, that an agent reconstructs itself from its arcs before
// turn one, and nothing had ever invoked it.
//
// It must be run from an INSTALLED layout. The script derives PROJECT_DIR from
// its own location ($SCRIPT_DIR/../..), so the copy in this repo resolves to
// the repo root, finds internal/ and cmd/, and takes a dev-loop branch that
// builds /tmp/vaultmind and ignores PATH. No adopter has that shape. Running
// the repo copy would test a path nobody ships.
func installPersonaHook(t *testing.T, stub string) (project string, env []string) {
	t.Helper()
	project = t.TempDir()
	scripts := filepath.Join(project, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "vaultmind-identity"), 0o750))

	body, ok := hookscripts.All()["load-persona.sh"]
	require.True(t, ok, "load-persona.sh must be embedded")
	require.NoError(t, os.WriteFile(filepath.Join(scripts, "load-persona.sh"), body, 0o700)) //nolint:gosec // test fixture

	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vaultmind"), []byte(stub), 0o700)) //nolint:gosec // test fixture

	home := t.TempDir()
	return project, []string{
		"PATH=" + binDir + ":/usr/bin:/bin",
		"HOME=" + home,
		"CLAUDE_PROJECT_DIR=" + project,
		"VAULTMIND_VAULT=" + filepath.Join(project, "vaultmind-identity"),
	}
}

func runPersonaHook(t *testing.T, project string, env []string) string {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	payload, err := json.Marshal(map[string]string{
		"session_id": "test", "hook_event_name": "SessionStart", "source": "startup",
	})
	require.NoError(t, err)

	cmd := exec.Command(bashPath, filepath.Join(project, ".claude", "scripts", "load-persona.sh"))
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(payload))
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoError(t, cmd.Run(), "hook must exit 0 (stderr: %s)", errb.String())
	return out.String()
}

const personaReciteStub = "#!/bin/bash\n" +
	"if [ \"$1\" = \"arc\" ] && [ \"$2\" = \"recite\" ]; then\n" +
	"  echo 'YOUR ARCS — 27 of 27, the moments that made you who you are.'\n" +
	"  echo 'SENTINEL-ARC-BODY'\n" +
	"  exit 0\n" +
	"fi\n" +
	"if [ \"$1\" = ask ]; then echo '  0.42  identity-who-i-am   Who I Am'; fi\n" +
	"exit 0\n"

// #47: the arc layer must arrive, as bodies, every session.
func TestPersonaHook_InjectsTheWholeArcLayer(t *testing.T) {
	project, env := installPersonaHook(t, personaReciteStub)

	out := runPersonaHook(t, project, env)

	assert.Contains(t, out, "YOUR ARCS — 27 of 27",
		"the persona hook must inject the ENUMERATED arc layer, not only the ranked ask")
	assert.Contains(t, out, "SENTINEL-ARC-BODY",
		"arcs must arrive as bodies; titles are the defect #47 describes")
	assert.Contains(t, out, "IDENTITY CONTEXT:",
		"the existing identity block must survive alongside the arc layer")
}

// The arc pass is best-effort: if it fails, the session still gets its
// identity. A bootstrap that loses everything because one query broke would be
// a worse failure than the one being fixed.
func TestPersonaHook_ArcFailureDoesNotCostTheIdentityBlock(t *testing.T) {
	stub := "#!/bin/bash\n" +
		"if [ \"$1\" = \"arc\" ]; then exit 3; fi\n" +
		"if [ \"$1\" = ask ]; then echo '  0.42  identity-who-i-am   Who I Am'; fi\n" +
		"exit 0\n"
	project, env := installPersonaHook(t, stub)

	out := runPersonaHook(t, project, env)

	assert.Contains(t, out, "IDENTITY CONTEXT:", "a failed arc pass must not suppress the identity")
	assert.NotContains(t, out, "YOUR ARCS")
}
