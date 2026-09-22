package query

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/doctorclient"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-22): an operator passed agents.yaml — the ROSTER — as
// --mesh-registry. Doctor answered "did not verify against your pinned root
// (bad signature, stale, or rolled back)". None of those was true; the file was
// never a signed registry. The wording sent the reader looking for tampering
// when the fix was a different path.

const rosterYAML = `daemon_url: http://100.64.22.69:8080
agents:
  - slug: mira
    display_name: Mira
`

func TestMeshDoctor_RosterPassedAsRegistryIsNamedAsSuch(t *testing.T) {
	rootPub, _ := lowEntropyKey(t, "not-a-registry-root")
	for name, in := range map[string]MeshDoctorInput{
		"pinned": {PinnedRootPub: rootPub},
		// The live shape: the daemon advertises its root, so the unpinned
		// self-consistency check runs against the roster bytes.
		"unpinned": {Daemon: &stubDaemon{
			root:     doctorclient.WellKnownRoot{RootPubKey: b64(rootPub), NetworkID: "n"},
			whoamiOK: true,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			in.KeyPath = filepath.Join(t.TempDir(), "k.key")
			in.SocketPath = filepath.Join(t.TempDir(), "missing.sock")
			in.RegistryBytes = []byte(rosterYAML)
			in.Slug = "agent:mira"
			in.Now = time.Unix(1_700_000_000, 0)

			mi, err := BuildMeshIdentity(context.Background(), in)
			require.NoError(t, err)
			require.Contains(t, mi.Warnings, WarnMeshRegistryNotSigned)
			require.NotContains(t, mi.Warnings, WarnMeshUnverifiable,
				"a file that was never a signed registry is not a bad signature")
			require.NotContains(t, mi.Warnings, WarnMeshSelfConsistencyFailed)
		})
	}
}
