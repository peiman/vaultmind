package personahealth

import (
	"encoding/json"
	"fmt"
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

func TestCheck_ArcMomentCoverageReportsPresentSections(t *testing.T) {
	report := Check(Config{Keepers: []Keeper{{
		Name: "moment", Transcripts: fixturePath("transcripts/full.jsonl"), Vault: fixturePath("vault-full"),
	}}, MinCoverage: 0.9}).Keepers[0]

	require.Len(t, report.Arcs.Items, 1)
	sections := report.Arcs.Items[0].Sections
	require.Len(t, sections, 2)
	assert.Equal(t, "trigger", sections[0].Name)
	assert.Zero(t, sections[0].ArrivedBytes)
	assert.Equal(t, "principle", sections[1].Name)
	assert.Greater(t, sections[1].Bytes, 0)
	assert.Greater(t, sections[1].ArrivedBytes, 0)
	assert.NotContains(t, []string{"push"}, sections[0].Name)
	assert.Equal(t, report.Arcs.MomentBytes, sections[0].Bytes+sections[1].Bytes)
	assert.Equal(t, report.Arcs.MomentArrivedBytes, sections[0].ArrivedBytes+sections[1].ArrivedBytes)
	assert.Greater(t, report.Arcs.MomentCoverage, 0.0)
	assert.Less(t, report.Arcs.MomentCoverage, 1.0)
}

func TestCheck_ArcMomentThresholdWarnsButNeverFails(t *testing.T) {
	report := Check(Config{Keepers: []Keeper{{
		Name: "moment", Transcripts: fixturePath("transcripts/full.jsonl"), Vault: fixturePath("vault-full"),
	}}, MinCoverage: 0.9, MinMomentCoverage: 1}).Keepers[0]

	assert.Equal(t, "warn", report.Status)
	assert.Contains(t, report.Warning, "arc moment coverage")
	assert.Empty(t, report.Error)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "arcs moment ")
}

func TestCheck_ArcMomentCoverageReportsEveryMomentSectionInOrder(t *testing.T) {
	report := Check(Config{Keepers: []Keeper{{
		Name: "moment", Transcripts: fixturePath("transcripts/moment-partial.jsonl"), Vault: fixturePath("vault-moment"),
	}}, MinCoverage: 0.9}).Keepers[0]

	require.Len(t, report.Arcs.Items, 2)
	alpha := report.Arcs.Items[0]
	assert.Equal(t, "arc-alpha", alpha.ID)
	require.Len(t, alpha.Sections, 3)
	assert.Equal(t, []string{"trigger", "push", "principle"}, []string{alpha.Sections[0].Name, alpha.Sections[1].Name, alpha.Sections[2].Name})
	assert.Equal(t, 1.0, alpha.Sections[0].Coverage)
	assert.Zero(t, alpha.Sections[1].ArrivedBytes)
	assert.Zero(t, alpha.Sections[1].Coverage)
	assert.Equal(t, 1.0, alpha.Sections[2].Coverage)

	beta := report.Arcs.Items[1]
	assert.Equal(t, "arc-beta", beta.ID)
	assert.NotNil(t, beta.Sections)
	assert.Empty(t, beta.Sections)
	assert.Equal(t, alpha.Sections[0].Bytes+alpha.Sections[1].Bytes+alpha.Sections[2].Bytes, report.Arcs.MomentBytes)
	assert.Equal(t, alpha.Sections[0].ArrivedBytes+alpha.Sections[1].ArrivedBytes+alpha.Sections[2].ArrivedBytes, report.Arcs.MomentArrivedBytes)
	assert.Equal(t, float64(report.Arcs.MomentArrivedBytes)/float64(report.Arcs.MomentBytes), report.Arcs.MomentCoverage)
	assert.NotContains(t, alpha.Sections, ArcSectionReport{Name: "deeper-sight"})

	data, err := json.Marshal(Result{Keepers: []Report{report}})
	require.NoError(t, err)
	var decoded struct {
		Keepers []struct {
			Arcs struct {
				Items          []map[string]json.RawMessage `json:"items"`
				MomentCoverage json.RawMessage              `json:"moment_coverage"`
			} `json:"arcs"`
		} `json:"keepers"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Keepers, 1)
	require.Len(t, decoded.Keepers[0].Arcs.Items, 2)
	for _, item := range decoded.Keepers[0].Arcs.Items {
		sections, ok := item["sections"]
		require.True(t, ok)
		assert.NotNil(t, sections)
	}
	assert.JSONEq(t, `[]`, string(decoded.Keepers[0].Arcs.Items[1]["sections"]))
	assert.NotNil(t, decoded.Keepers[0].Arcs.MomentCoverage)
}

func TestCheck_ArcMomentCoverageThresholdOnlyWarnsAboveMeasuredFraction(t *testing.T) {
	keeper := Keeper{Name: "moment", Transcripts: fixturePath("transcripts/moment-partial.jsonl"), Vault: fixturePath("vault-moment")}
	baseline := Check(Config{Keepers: []Keeper{keeper}, MinCoverage: 0.9}).Keepers[0]
	coverage := baseline.Arcs.MomentCoverage

	above := Check(Config{Keepers: []Keeper{keeper}, MinCoverage: 0.9, MinMomentCoverage: coverage + 0.0001}).Keepers[0]
	assert.Equal(t, "warn", above.Status)
	assert.Contains(t, above.Warning, "arc moment coverage")
	assert.Empty(t, above.Error)

	for _, threshold := range []float64{coverage, coverage - 0.0001} {
		atOrBelow := Check(Config{Keepers: []Keeper{keeper}, MinCoverage: 0.9, MinMomentCoverage: threshold}).Keepers[0]
		assert.NotContains(t, atOrBelow.Warning, "arc moment coverage")
	}

	wantPercent := int(float64(baseline.Arcs.MomentArrivedBytes) * 100 / float64(baseline.Arcs.MomentBytes))
	assert.Contains(t, Text(Result{Keepers: []Report{baseline}}), fmt.Sprintf("arcs moment %d%%", wantPercent))
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

func TestCheck_MemoryNotSeenAfterCompactionWarns(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-not-seen",
		Transcripts: fixturePath("transcripts/memory-not-seen-after-reload.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "warn", report.Status)
	assert.False(t, report.Memory.Seen)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "memory not seen")
	assert.Contains(t, report.Warning, "memory file not seen after compaction at 2026-11-24 10:00Z")
	assert.Empty(t, report.Error)

	data, err := json.Marshal(report)
	require.NoError(t, err)
	var decoded struct {
		Memory struct {
			Seen bool `json:"seen"`
		} `json:"memory"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.False(t, decoded.Memory.Seen)
}

func TestCheck_LaterNonMemoryInstructionsPreserveTruncation(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-preserved",
		Transcripts: fixturePath("transcripts/memory-truncated-then-project.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Seen)
	assert.True(t, report.Memory.Truncated)
	assert.Equal(t, 200, report.Memory.LoadedLines)
	assert.Equal(t, 202, report.Memory.TotalLines)
	assert.Equal(t, 2, report.Memory.CutLines)
	assert.Contains(t, report.Memory.Notice, "Only part of it was loaded")
	assert.Contains(t, report.Error, "memory truncated")
}

func TestCheck_CompactionResetsEarlierSessionStartText(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "stale",
		Transcripts: fixturePath("transcripts/precompact-full-postcompact-title.jsonl"),
		Vault:       fixturePath("vault-title"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Zero(t, report.Identity.Coverage)
	assert.Zero(t, report.Identity.CoveredBytes)
}

func TestCheck_TruncatedCompactBoundaryClearsEarlierIdentity(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "truncated-boundary",
		Transcripts: fixturePath("transcripts/truncated-compact-boundary.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	data, err := json.Marshal(report)
	require.NoError(t, err)
	var got struct {
		UnreadableLines int `json:"unreadable_lines"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "fail", report.Status)
	assert.Zero(t, report.Identity.CoveredBytes)
	assert.Equal(t, 1, report.UnreadableLines)
	assert.Equal(t, 1, got.UnreadableLines)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "1 unreadable line")
	assert.Equal(t, "unknown", report.AfterCompact)
}

func TestCheck_OnlySessionStartHookSuccessCountsAfterCompaction(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "wrong-records",
		Transcripts: fixturePath("transcripts/wrong-session-start-records.jsonl"),
		Vault:       fixturePath("vault-title"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Zero(t, report.Identity.Coverage)
	assert.Zero(t, report.Identity.CoveredBytes)
}

func TestCheck_MemoryTruncationFailsAndReportsCounts(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory",
		Transcripts: fixturePath("transcripts/memory-truncated.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Equal(t, 200, report.Memory.LoadedLines)
	assert.Equal(t, 202, report.Memory.TotalLines)
	assert.Equal(t, 2, report.Memory.CutLines)
	assert.Contains(t, report.Error, "memory truncated")
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "truncated (loaded 200 of 202 lines, 2 cut off)")
}

func TestCheck_MemoryTruncationLinesAndKBVariantFails(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-lines-and-kb",
		Transcripts: fixturePath("transcripts/memory-truncated-lines-and-kb.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Equal(t, 249, report.Memory.TotalLines)
	assert.Equal(t, 49, report.Memory.CutLines)
	assert.Equal(t, 200, report.Memory.LoadedLines)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "truncated (loaded 200 of 249 lines, 49 cut off)")
}

func TestCheck_MemoryTruncationWithoutLineNumbersFailsWithUnknownCounts(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-kb-only",
		Transcripts: fixturePath("transcripts/memory-truncated-kb-only.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Zero(t, report.Memory.TotalLines)
	assert.Zero(t, report.Memory.CutLines)
	assert.Zero(t, report.Memory.LoadedLines)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "truncated (numbers unknown)")

	data, err := json.Marshal(report.Memory)
	require.NoError(t, err)
	var encoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &encoded))
	assert.NotContains(t, encoded, "loaded_lines")
	assert.NotContains(t, encoded, "total_lines")
	assert.NotContains(t, encoded, "cut_lines")
}

func TestCheck_MemoryTruncationWithoutCompactionFails(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-without-compaction",
		Transcripts: fixturePath("transcripts/memory-truncated-without-compaction.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Zero(t, report.Memory.LoadedLines)
	assert.Zero(t, report.Memory.TotalLines)
	assert.Zero(t, report.Memory.CutLines)
	assert.Contains(t, Text(Result{Keepers: []Report{report}}), "truncated (numbers unknown)")
}

func TestCheck_MemoryTruncationBeforeLastCompactionIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory-reset",
		Transcripts: fixturePath("transcripts/memory-before-last-compaction.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "warn", report.Status)
	assert.False(t, report.Memory.Truncated)
	assert.Empty(t, report.Error)
}

func TestCheck_MemoryNoticeInAssistantAndToolResultIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "quoted-memory",
		Transcripts: fixturePath("transcripts/memory-notice-quoted.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_QueuedCommandAttachmentQuotingMemoryNoticeIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "queued-command-quoted-memory",
		Transcripts: fixturePath("transcripts/memory-queued-command-quoted.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_HookAdditionalContextAttachmentQuotingMemoryNoticeIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "hook-context-quoted-memory",
		Transcripts: fixturePath("transcripts/memory-hook-additional-context-quoted.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_RenderedAttachmentMemoryNoticeIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "rendered-memory",
		Transcripts: fixturePath("transcripts/memory-notice-rendered.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_APIRequestBlobMemoryNoticeIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "api-request-blob-memory",
		Transcripts: fixturePath("transcripts/memory-notice-api-request-blob.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_OldMemoryNoticeParaphraseIsIgnored(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "old-memory-notice",
		Transcripts: fixturePath("transcripts/memory-old-paraphrase.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
}

func TestCheck_AutoMemInstructionsContainingNoticeFailsMemory(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "memory",
		Transcripts: fixturePath("transcripts/memory-truncated.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.True(t, report.Memory.Truncated)
	assert.Equal(t, 200, report.Memory.LoadedLines)
	assert.Equal(t, 202, report.Memory.TotalLines)
	assert.Equal(t, 2, report.Memory.CutLines)
	assert.Equal(t, []MemoryFileReport{{
		Path:  "/fictional/.claude/projects/demo/memory.md",
		Type:  "AutoMem",
		Lines: 4,
	}}, report.Memory.Files)
	data, err := json.Marshal(report)
	require.NoError(t, err)
	var decoded struct {
		Memory struct {
			Files []MemoryFileReport `json:"files"`
		} `json:"memory"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, report.Memory.Files, decoded.Memory.Files)
}

func TestCheck_UserAndProjectFilesContainingNoticeDoNotCount(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "non-memory-files",
		Transcripts: fixturePath("transcripts/memory-user-project-notice.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
	assert.True(t, report.Memory.Seen)
	assert.Len(t, report.Memory.Files, 1)
	assert.Equal(t, "AutoMem", report.Memory.Files[0].Type)
}

func TestCheck_CleanAutoMemFileReportsLineCount(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "clean-memory",
		Transcripts: fixturePath("transcripts/memory-clean.jsonl"),
		Vault:       fixturePath("vault-full"),
	}, 0.9)

	assert.Equal(t, "ok", report.Status)
	assert.False(t, report.Memory.Truncated)
	assert.Equal(t, []MemoryFileReport{{
		Path:  "/fictional/.claude/projects/demo/memory.md",
		Type:  "AutoMem",
		Lines: 2,
	}}, report.Memory.Files)
}

func TestCheck_EmptySessionStartContentDoesNotReload(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "empty-reload",
		Transcripts: fixturePath("transcripts/empty-session-start.jsonl"),
		Vault:       fixturePath("vault-title"),
	}, 0.9)

	assert.Equal(t, "fail", report.Status)
	assert.Zero(t, report.Identity.Coverage)
	assert.Contains(t, report.Error, "NO RELOAD")
}

func TestCheck_SkipsMalformedJSONLAmongValidRecords(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "malformed-between",
		Transcripts: fixturePath("transcripts/malformed-between-valid.jsonl"),
		Vault:       fixturePath("vault-title"),
	}, 0.9)

	assert.Equal(t, "warn", report.Status)
	assert.Equal(t, 1.0, report.Identity.Coverage)
}

func TestReadTranscriptRejectsFileWithoutValidRecords(t *testing.T) {
	_, err := readTranscript(fixturePath("transcripts/unreadable.jsonl"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid records")
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

func TestText_MemoryWithoutNoticeSaysMemoryIsOK(t *testing.T) {
	report := CheckKeeper(Keeper{
		Name:        "clean-memory",
		Transcripts: fixturePath("transcripts/memory-clean.jsonl"),
		Vault:       fixturePath("vault-memory"),
	}, 0.9)

	require.False(t, report.Memory.Truncated)
	assert.True(t, report.Memory.NoticeProven)
	text := Text(Result{Keepers: []Report{report}})
	assert.Contains(t, text, "memory ok")
	assert.NotContains(t, text, "not proven")
}
