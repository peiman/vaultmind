package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A vault path that does not exist must not read as "no arc material".
//
// `arc candidates` scans directories directly rather than opening the vault DB,
// so it never noticed a bad --vault: a typo produced "Scanned 0 episodes → 0
// candidate moments", exit 0 — identical to a real vault with nothing pending.
// The two call for opposite responses (fix the path vs. go write something), so
// they must not share a representation. Same failure this session fixed in
// `ask`, found in the command being extended to read the desk.
func TestArcCandidates_NonexistentVaultFailsClosed(t *testing.T) {
	_, _, err := runRootCmd(t, "arc", "candidates", "--vault", "/does/not/exist")
	require.Error(t, err, "a missing vault must not report an empty backlog")
	assert.Contains(t, err.Error(), "/does/not/exist")
}

// A real vault with no episodes and no desk is genuinely empty — that stays a
// success, because "nothing pending" is a legitimate answer.
func TestArcCandidates_RealButEmptyVaultSucceeds(t *testing.T) {
	_, _, err := runRootCmd(t, "arc", "candidates", "--vault", t.TempDir())
	require.NoError(t, err)
}

// The desk must reach the JSON envelope too — an agent consuming --json is the
// primary reader of this command, and a section that exists only in the human
// text is invisible to it.
func TestArcCandidates_JSONCarriesDeskEntries(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "journal"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(vault, "journal", "e.md"),
		[]byte("---\nid: journal-e\ntype: journal\ndate: 2026-08-13\ntitle: An entry\n---\n\nBody.\n"),
		0o600))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			DeskPending []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"desk_pending"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Len(t, env.Result.DeskPending, 1)
	assert.Equal(t, "journal-e", env.Result.DeskPending[0].ID)
	assert.Equal(t, "An entry", env.Result.DeskPending[0].Title)
	// The --json consumer is the primary reader of this command, so the shape
	// of what it receives is part of the contract: tagging a section `json:"-"`
	// must fail a test rather than silently emptying the payload.
	assert.Contains(t, out.String(), `"desk_pending"`)
}

// The de-duplication aid must degrade, never take the report down with it: a
// vault that can't be opened for arc comparison still yields the proposals,
// with the reason recorded.
func TestOpenArcFinder_MissingVaultErrors(t *testing.T) {
	_, _, err := openArcFinder("/does/not/exist")
	require.Error(t, err)
}

func TestArcCandidates_UnindexedArcsVaultStillReports(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "journal"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "journal", "e.md"),
		[]byte("---\nid: journal-e\ntype: journal\ndate: 2026-08-13\ntitle: An entry\n---\n\nBody.\n"), 0o600))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault,
		"--arcs-vault", "/does/not/exist", "--json")
	require.NoError(t, err, "the proposals survive a failed neighbour lookup")

	var env struct {
		Result struct {
			DeskPending []struct{ ID string } `json:"desk_pending"`
			Diagnostics []string              `json:"diagnostics"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Len(t, env.Result.DeskPending, 1)
	assert.NotEmpty(t, env.Result.Diagnostics, "the degradation is reported, not silent")
}

// REGRESSION (found by review 2026-08-15): `arc candidates` must not create a
// vault where it was merely pointed.
//
// v0.3.0 fixed exactly this in the read path — OpenVaultDB CREATES
// .vaultmind/index.db under whatever path it is given, promoting that directory
// to a vault every later walk-up finds. This command reintroduced it by calling
// the raw opener for the arcs vault, and `--arcs-vault` had no path validation
// at all.
func TestArcCandidates_DoesNotCreateAVaultWhereItLooks(t *testing.T) {
	scanned := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(scanned, ".vaultmind"), 0o750))
	plainDir := t.TempDir() // exists, but is NOT a vault

	_, _, _ = runRootCmd(t, "arc", "candidates", "--vault", scanned, "--arcs-vault", plainDir)

	_, err := os.Stat(filepath.Join(plainDir, ".vaultmind"))
	assert.True(t, os.IsNotExist(err),
		"pointing the arc comparison at a directory must not turn that directory into a vault")
}

// A --arcs-vault that doesn't exist is a typo in the flag whose entire purpose
// is de-duplication. It must be reported, not silently ignored.
func TestArcCandidates_BadArcsVaultIsReportedInHumanOutput(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "journal"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "journal", "e.md"),
		[]byte("---\nid: journal-e\ntype: journal\ndate: 2026-08-13\ntitle: An entry\n---\n\nBody.\n"), 0o600))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--arcs-vault", "/does/not/exist")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "de-duplication",
		"the human report must say the aid was unavailable; only JSON carrying it is not enough")
}

// The --arcs-vault HAPPY path: desk in one vault, arcs in another. Only the
// failure case was covered, so deleting the "empty means use the scanned vault"
// fallback passed CI while leaving the aid permanently inert.
func TestArcCandidates_CrossVaultNeighboursAreFound(t *testing.T) {
	deskVault := buildIndexedTestVault(t)
	require.NoError(t, os.MkdirAll(filepath.Join(deskVault, "journal"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(deskVault, "journal", "e.md"),
		[]byte("---\nid: journal-e\ntype: journal\ndate: 2026-08-15\ntitle: An entry\n---\n\nBody.\n"), 0o600))

	// A separate, unembedded arcs vault: the aid must report that it could not
	// run, rather than silently returning no neighbours.
	arcsVault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", deskVault, "--arcs-vault", arcsVault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			DeskPending []struct {
				ID          string `json:"id"`
				NearestArcs []struct {
					ID    string  `json:"id"`
					Score float64 `json:"score"`
				} `json:"nearest_arcs"`
			} `json:"desk_pending"`
			Diagnostics []string `json:"diagnostics"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Len(t, env.Result.DeskPending, 1, "the desk entry is reported from the scanned vault")
	assert.NotEmpty(t, env.Result.Diagnostics,
		"the reader must be told the aid did not run, or they will read no neighbours as 'nothing resembles this'")
	assert.Equal(t, "ok", env.Status,
		"a vault without embeddings is a CONFIGURATION, not a fault: warning here would fire on an ordinary setup and teach the reader to ignore warnings")
}

// The other side of that line: something actually broken must reach a caller
// gating on status. A mistyped --arcs-vault is the flag whose entire purpose is
// de-duplication, so it cannot pass as an ordinary run.
func TestArcCandidates_RealFailureSetsWarningStatus(t *testing.T) {
	vault := buildIndexedTestVault(t)
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "journal"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "journal", "e.md"),
		[]byte("---\nid: journal-e\ntype: journal\ndate: 2026-08-15\ntitle: An entry\n---\n\nBody.\n"), 0o600))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault,
		"--arcs-vault", "/does/not/exist", "--json")
	require.NoError(t, err)

	var env struct {
		Status   string `json:"status"`
		Warnings []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "warning", env.Status, "a bad path is broken, not merely unconfigured")
	assert.NotEmpty(t, env.Warnings)
}
