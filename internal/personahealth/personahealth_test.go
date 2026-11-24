package personahealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}

func TestCheck_FullDeliveryIsOK(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "full",
		Transcripts: fixturePath("transcripts/full.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.Equal(t, 1.0, report.Identity.Coverage)
	assert.Equal(t, report.Identity.TotalBytes, report.Identity.CoveredBytes)
	assert.Equal(t, 1, report.Arcs.Full)
	assert.Zero(t, report.Arcs.Partial)
	assert.Zero(t, report.Arcs.Missing)
	assert.Len(t, report.Arcs.Items, 1)
	assert.Equal(t, "arc-boundary", report.Arcs.Items[0].ID)
	assert.Greater(t, report.Arcs.BodyBytes, 0)
	assert.Greater(t, report.Arcs.ArrivedBytes, 0)
	assert.Less(t, report.Arcs.ArrivedBytes, report.Arcs.BodyBytes)
	assert.Greater(t, report.Arcs.Coverage, 0.0)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_ArcBodyCoverageUsesEveryBodySentenceAndExcludesFrontmatter(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "body",
		Transcripts: fixturePath("transcripts/arc-body.jsonl"),
		Vault:       fixturePath("vault-body"),
	}, 0.9)

	assert.Equal(t, "warn", report.Status)
	require.Len(t, report.Arcs.Items, 2)
	assert.Equal(t, "arc-first", report.Arcs.Items[0].ID)
	assert.Equal(t, "arc-second", report.Arcs.Items[1].ID)
	assert.Equal(t, 38, report.Arcs.Items[0].BodyBytes)
	assert.Equal(t, 26, report.Arcs.Items[0].ArrivedBytes)
	assert.Equal(t, 40, report.Arcs.Items[1].BodyBytes)
	assert.Equal(t, 0, report.Arcs.Items[1].ArrivedBytes)
	assert.Equal(t, 78, report.Arcs.BodyBytes)
	assert.Equal(t, report.Arcs.Items[0].ArrivedBytes, report.Arcs.ArrivedBytes)
}

func TestCheck_ArcCoverageThresholdWarnsButNeverFails(t *testing.T) {
	keeper := Keeper{
		Name:        "body",
		Transcripts: fixturePath("transcripts/arc-body.jsonl"),
		Vault:       fixturePath("vault-body"),
	}
	report := Check(Config{Keepers: []Keeper{keeper}, MinCoverage: 0.9, MinArcCoverage: 0.8}).Keepers[0]

	assert.Equal(t, "warn", report.Status)
	assert.Contains(t, report.Warning, "arc body coverage")
	assert.Empty(t, report.Error)

}

func TestCheck_TitleOnlyIdentityFailsCoverage(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "title",
		Transcripts: fixturePath("transcripts/title-only.jsonl"),
		Vault:       fixturePath("vault-title"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Zero(t, report.Identity.CoveredBytes)
	assert.NotEmpty(t, report.Error)
	assert.Equal(t, 0, report.Arcs.Full)
	assert.Greater(t, report.Arcs.Partial, 0)
}

func TestCheck_PartialIdentityCanWarnWhenThresholdAllowsIt(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "partial",
		Transcripts: fixturePath("transcripts/partial.jsonl"),
		Vault:       fixturePath("vault-partial"),
	}, 0.1)

	assert.Equal(t, "warn", report.Status)
	assert.Less(t, report.Identity.Coverage, 1.0)
	assert.Equal(t, 1, report.Arcs.Partial)
}

func TestCheck_NoReloadAfterCompactionFails(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "noreload",
		Transcripts: fixturePath("transcripts/no-reload.jsonl"),
		Vault:       fixturePath("vault-noreload"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Contains(t, report.Error, "NO RELOAD after compaction at 2026-11-24 10:00Z")
	assert.Equal(t, "2026-11-24 10:00Z", report.AfterCompact)
}

func TestCheck_MemoryTruncationFailsAndReportsCounts(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory",
		Transcripts: fixturePath("transcripts/memory-truncated.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Equal(t, 46, report.Memory.LoadedLines)
	assert.Equal(t, 246, report.Memory.TotalLines)
	assert.Contains(t, report.Error, "memory truncated")
}

func TestCheck_UnreadableTranscriptFailsPerKeeper(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "unreadable",
		Transcripts: fixturePath("transcripts/unreadable"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Contains(t, report.Error, "transcript")
}

func TestLoadKeepers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keepers.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
- name: mira
  transcripts: /tmp/mira
  vault: /tmp/mira-vault
  anchor: identity-who-i-am
`), 0o600))

	keepers, err := LoadKeepers(path)
	require.NoError(t, err)
	require.Len(t, keepers, 1)
	assert.Equal(t, "mira", keepers[0].Name)
	assert.Equal(t, "identity-who-i-am", keepers[0].Anchor)
}

func TestCheckAndTextRenderMultipleKeepers(t *testing.T) {
	result := Check(Config{Keepers: []Keeper{
		{Name: "full", Transcripts: fixturePath("transcripts/full.jsonl"), Vault: fixturePath("vault-full")},
		{Name: "title", Transcripts: fixturePath("transcripts/title-only.jsonl"), Vault: fixturePath("vault-title")},
	}, MinCoverage: 0.9})

	require.Len(t, result.Keepers, 2)
	text := Text(result)
	assert.Contains(t, text, "full  OK")
	assert.Contains(t, text, "title  FAIL")
	assert.Equal(t, 2, strings.Count(strings.TrimSpace(text), "\n")+1)
	assert.Equal(t, "", Text(Result{}))
}

func TestKeepersFromFlagsEnforcesExactlyOneForm(t *testing.T) {
	keepers, err := KeepersFromFlags("", "transcripts", "vault", "", "")
	require.NoError(t, err)
	assert.Equal(t, "transcripts", keepers[0].Name)
	keepers, err = KeepersFromFlags("mira", "transcripts", "vault", "", "")
	require.NoError(t, err)
	assert.Equal(t, "mira", keepers[0].Name)
	_, err = KeepersFromFlags("", "", "", "", fixturePath("missing-keepers.yaml"))
	assert.Error(t, err)
	_, err = KeepersFromFlags("", "transcripts", "", "", "")
	assert.Error(t, err)
	_, err = KeepersFromFlags("", "transcripts", "vault", "", fixturePath("missing-keepers.yaml"))
	assert.Error(t, err)
	_, err = KeepersFromFlags("mira", "", "", "", fixturePath("missing-keepers.yaml"))
	assert.Error(t, err)
	assert.Error(t, ValidateMinCoverage(-0.1))
	assert.Error(t, ValidateMinCoverage(1.1))
	assert.NoError(t, ValidateMinCoverage(0.9))
	assert.Error(t, ValidateMinArcCoverage(-0.1))
	assert.Error(t, ValidateMinArcCoverage(1.1))
	assert.NoError(t, ValidateMinArcCoverage(0.9))
}

func TestKeepersFromFlagsDefaultsNameToTranscriptDirectoryBase(t *testing.T) {
	keepers, err := KeepersFromFlags("", "/Users/peiman/.claude/projects/-Users-peiman-dev-cli-vaultmind", "/tmp/identity", "", "")
	require.NoError(t, err)
	assert.Equal(t, "-Users-peiman-dev-cli-vaultmind", keepers[0].Name)
}

func TestTextIncludesArcBodyTotalsButNotEachArc(t *testing.T) {
	result := Result{Keepers: []Report{{
		Name:   "body",
		Status: "ok",
		Arcs: ArcReport{
			Full:         1,
			BodyBytes:    29231,
			ArrivedBytes: 1902,
			Items:        []ArcBodyReport{{ID: "arc-first", BodyBytes: 100, ArrivedBytes: 50}},
		},
	}}}

	text := Text(result)
	assert.Contains(t, text, "arcs 1 full / 0 partial / 0 missing")
	assert.Contains(t, text, "arcs body 1,902 of 29,231 bytes, 6%")
	assert.NotContains(t, text, "arc arc-first body 50 of 100 bytes, 50%")
	assert.Equal(t, 1, strings.Count(strings.TrimSpace(text), "\n")+1)
}

func TestReadTranscriptChoosesNewestAndSupportsSessionStart(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.jsonl")
	newest := filepath.Join(dir, "new.jsonl")
	require.NoError(t, os.WriteFile(old, []byte(`{"type":"system","timestamp":"2026-11-24T09:00:00Z"}
{"type":"attachment","timestamp":"2026-11-24T09:00:01Z","attachment":{"type":"hook_success","hookEvent":"SessionStart","content":"old"}}
`), 0o600))
	require.NoError(t, os.WriteFile(newest, []byte(`{"type":"system","timestamp":"2026-11-24T11:00:00Z"}
{"type":"attachment","timestamp":"2026-11-24T11:00:01Z","attachment":{"type":"hook_success","hookEvent":"SessionStart","content":"new"}}
`), 0o600))
	oldTime := time.Unix(1, 0)
	newTime := time.Unix(2, 0)
	require.NoError(t, os.Chtimes(old, oldTime, oldTime))
	require.NoError(t, os.Chtimes(newest, newTime, newTime))
	evidence, err := readTranscript(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, evidence.Texts)
	assert.Equal(t, "after session start", evidence.PointPrefix)
}

func TestReadTranscriptAcceptsArrayAttachmentContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "array.jsonl")
	content := `{"type":"attachment","timestamp":"2026-11-24T11:00:00Z","attachment":{"type":"hook_success","hookEvent":"SessionStart","content":[{"type":"text","text":"array content"}]}}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	evidence, err := readTranscript(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"array content"}, evidence.Texts)
}

func TestCheckReportsMissingVaultAndMissingTranscript(t *testing.T) {
	transcriptReport := CheckKeeper(Keeper{Name: "missing", Transcripts: fixturePath("missing-transcripts"), Vault: fixturePath("vault-full")}, 0.9)
	assert.Equal(t, "fail", transcriptReport.Status)
	vaultReport := CheckKeeper(Keeper{Name: "missing", Transcripts: fixturePath("transcripts/full.jsonl"), Vault: fixturePath("missing-vault")}, 0.9)
	assert.Equal(t, "fail", vaultReport.Status)
	assert.Contains(t, vaultReport.Error, "vault")
}
