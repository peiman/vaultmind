package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"

	"github.com/stretchr/testify/require"
)

// Capability profiles exist because `hooks status` judged every adopter against
// everything the binary ships. Run against focalc's correctly-configured
// knowledge vault it reported 3 unwired, 1 missing — persona loading, episode
// capture and mesh-watch, every one a deliberate omission for a vault with no
// persona, no episodes and no mesh. A report that is wrong on every run trains
// the reader to skip the run where it is right.
//
// The profile is DECLARED, never inferred from what happens to be installed:
// inferring it would make the check vacuous (nothing could ever be missing),
// which is the same hollow-check class the tool keeps finding elsewhere.

func TestProfile_KnowledgeOmitsPersonaEpisodeAndMesh(t *testing.T) {
	scripts := ScriptsForProfile(ProfileKnowledge)
	require.Contains(t, scripts, hookUserPromptSubmitScript, "recall is the point of a knowledge vault")
	require.Contains(t, scripts, hookPreToolUseScript)
	require.Contains(t, scripts, hookReachScript)
	require.Contains(t, scripts, hookHealthScript)

	require.NotContains(t, scripts, hookSessionStartScript, "no persona to load")
	require.NotContains(t, scripts, hookSessionEndScript, "no episodes to capture")
	require.NotContains(t, scripts, "mesh-watch.sh", "no mesh to watch")
}

func TestProfile_FullIsEveryCanonicalScript(t *testing.T) {
	full := ScriptsForProfile(ProfileFull)
	canonical := CanonicalEventScripts()
	// Guard the loop: an empty canonical set would make every assertion below
	// vacuous and the test would pass by never running — the shape this repo's
	// zero-iteration meta-test exists to catch, and it caught this one.
	require.NotEmpty(t, canonical)
	for _, es := range canonical {
		require.Contains(t, full, es.Script, "the full profile must cover every canonical hook")
	}
}

func TestProfile_UndeclaredMeansFullSoExistingAdoptersAreUnaffected(t *testing.T) {
	dir := t.TempDir()
	p, err := DeclaredProfile(dir)
	require.NoError(t, err)
	require.Equal(t, ProfileFull, p, "back-compat: an adopter who declared nothing keeps today's behaviour")
}

func TestProfile_DeclaredIsRead(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o750))
	require.NoError(t, WriteDeclaredProfile(dir, ProfileKnowledge))

	p, err := DeclaredProfile(dir)
	require.NoError(t, err)
	require.Equal(t, ProfileKnowledge, p)
}

// An unrecognised profile is an ERROR, not a silent fallback to full. Falling
// back would report a typo'd profile as a healthy full install — the failure
// mode this whole feature exists to remove.
func TestProfile_UnknownDeclarationIsLoud(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", profileFilename), []byte("knowlege\n"), 0o600))

	_, err := DeclaredProfile(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "knowlege", "the bad value must appear so the operator can see the typo")
}

// The end-to-end shape focalc actually hits: a knowledge vault with recall
// installed and wired, and NO persona/episode/mesh. Before profiles this
// reported "3 unwired, 1 missing" — telling a correct adopter they are broken.
func TestStatus_KnowledgeVaultWithNoPersonaOrMeshIsHealthy(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	require.NoError(t, WriteDeclaredProfile(dir, ProfileKnowledge))

	// Install exactly the knowledge set, byte-identical to canonical.
	for _, name := range ScriptsForProfile(ProfileKnowledge) {
		body, ok := hookscripts.Get(name)
		require.True(t, ok, "canonical script %s must exist", name)
		require.NoError(t, os.WriteFile(filepath.Join(scripts, name), body, 0o700))
	}
	stanza, err := SettingsStanzaForProfile(ProfileKnowledge, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(stanza), 0o600))

	report, serr := Status(dir)
	require.NoError(t, serr)
	_, drifted, missing := report.Counts()
	_, unwired := report.EventCounts()
	require.Zero(t, missing, "persona/episode/mesh are not missing from a knowledge vault; they are not part of it")
	require.Zero(t, unwired, "nor unwired")
	require.Zero(t, drifted)
}

// Installing with a profile must DECLARE it, or status has nothing to judge
// against and silently reverts to grading every adopter on the full inventory.
func TestInstall_ProfileIsDeclaredAndInstallsOnlyItsScripts(t *testing.T) {
	dir := t.TempDir()

	res, err := Install(InstallConfig{ProjectDir: dir, Profile: ProfileKnowledge})
	require.NoError(t, err)
	require.NotEmpty(t, res.Written)

	declared, err := DeclaredProfile(dir)
	require.NoError(t, err)
	require.Equal(t, ProfileKnowledge, declared, "the choice must survive the install that made it")

	for _, name := range res.Written {
		require.Contains(t, ScriptsForProfile(ProfileKnowledge), filepath.Base(name),
			"install must not write scripts outside the declared profile")
	}
	stanza, err := SettingsStanzaForProfile(ProfileKnowledge, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(stanza), 0o600))

	report, err := Status(dir)
	require.NoError(t, err)
	_, drifted, missing := report.Counts()
	_, unwired := report.EventCounts()
	require.Zero(t, missing+drifted+unwired, "an install of a profile must produce a clean status for that profile")
}
