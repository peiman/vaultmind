package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEmbedder is the minimum viable Embedder for label-only tests —
// retrievalModeLabel only reads nil-ness, so the methods don't need
// meaningful implementations.
type stubEmbedder struct{}

func (stubEmbedder) Embed(context.Context, string) ([]float32, error)          { return nil, nil }
func (stubEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) { return nil, nil }
func (stubEmbedder) Dims() int                                                 { return 0 }
func (stubEmbedder) Close() error                                              { return nil }

var _ embedding.Embedder = stubEmbedder{}

// doctor must surface the core vault health metrics. A doctor that silently
// drops one of these leaves the user debugging blind.
func TestDoctor_ReportsCoreMetrics(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			VaultPath         string `json:"vault_path"`
			TotalFiles        int    `json:"total_files"`
			DomainNotes       int    `json:"domain_notes"`
			UnstructuredNotes int    `json:"unstructured_notes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, vault, env.Result.VaultPath)
	assert.Equal(t, 4, env.Result.TotalFiles)
	assert.Equal(t, 3, env.Result.DomainNotes)
	assert.Equal(t, 1, env.Result.UnstructuredNotes)
}

// doctor human output must include the embedding-readiness line when
// embeddings are absent. That line is the single actionable suggestion
// a user gets from `doctor` for enabling semantic retrieval.
func TestDoctor_HumanOutputNamesEmbedRemedy(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Vault: ")
	assert.Contains(t, text, "Embeddings: none",
		"doctor must name 'none' when no embeddings exist — that's the remedy trigger")
	assert.Contains(t, text, "vaultmind index --embed",
		"doctor must print the exact remedy command users should run")
	assert.NotContains(t, text, "--model bge-m3",
		"the 'no embeddings' remedy must run on any backend — bge-m3 is refused on the pure-Go binary go install yields")
}

// doctor human output surfaces the Obsidian-incompatible link section when
// any are detected. Losing this printing would mean users who run `doctor`
// never see what's wrong — the diagnostic is there in JSON but invisible
// at the terminal.
func TestDoctor_HumanOutputSurfacesObsidianIncompatibleLinks(t *testing.T) {
	// buildIndexedTestVault has [[proj-beta]] → beta.md which creates an
	// incompatible link (proj-beta != beta).
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Obsidian-incompatible links",
		"the section header must appear when any exist")
	assert.Contains(t, text, "proj-beta",
		"the specific incompatible target must be named so the user can fix it")
}

// doctor surfaces hook-drift when CWD has installed scripts whose
// bytes differ from the embedded canonical. This is the "the
// foundation has rotted" detector — copies were edited or the binary
// was upgraded with old copies left in place. Resolution path is
// printed alongside (`vaultmind hooks install --force .`).
func TestDoctor_SurfacesHookDriftFromCWD(t *testing.T) {
	vault := buildIndexedTestVault(t)

	// Stage a fake project root with a drifted hook copy. Chdir to
	// it for the doctor run; doctor reads CWD to find .claude/scripts/.
	projectDir := t.TempDir()
	scriptsDir := filepath.Join(projectDir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(scriptsDir, "load-persona.sh"),
		[]byte("# DRIFTED COPY — not the canonical\n"),
		0o600,
	))

	// NOTE: os.Chdir is process-global; this test must NOT use
	// t.Parallel(). doctor walks up from CWD to find the project root
	// (the dir with .claude/), so chdir-ing into projectDir makes the
	// walk-up resolve here.
	origCWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origCWD) })
	require.NoError(t, os.Chdir(projectDir))

	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Hook drift",
		"doctor must surface a 'Hook drift' line when canonical bytes differ from installed copy")
	assert.Contains(t, text, "load-persona.sh",
		"the specific drifted hook must be named so the user can locate the bad copy")
	assert.Contains(t, text, "vaultmind hooks install --force",
		"doctor must print the exact remedy command")
}

// doctor surfaces a warning when `.claude/hooks.json` exists at the
// project root. That standalone file is silently broken on Claude
// Code 2.1.129+ (live evidence: companion-project dogfood May 5→May 6 2026).
// Without this check, the most user-visible failure shape we know of
// is invisible to operators — they think hooks are firing when they
// aren't.
func TestDoctor_SurfacesLegacyHooksJSONFromCWD(t *testing.T) {
	vault := buildIndexedTestVault(t)

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".claude", "hooks.json"),
		[]byte(`{"hooks":{"SessionStart":[]}}`),
		0o600,
	))

	// NOTE: os.Chdir is process-global; this test must NOT use
	// t.Parallel(). doctor walks up from CWD to find the project root.
	origCWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origCWD) })
	require.NoError(t, os.Chdir(projectDir))

	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Legacy hooks.json",
		"doctor must surface a 'Legacy hooks.json' warning when the silently-broken file exists")
	assert.Contains(t, text, "settings.json",
		"the resolution must name the migration target file")
}

// --summary on a clean vault (no incompatible links, no dead refs)
// should print just the headline counts — no extra noise, no empty
// "but you can see details with..." dangling line.
func TestDoctor_SummaryFlagOnCleanVaultIsTerse(t *testing.T) {
	// Build a tiny vault with no broken links — alpha and beta both
	// resolve correctly via aliases.
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--summary")
	require.NoError(t, err)
	text := out.String()
	// Headline lines always present.
	assert.Contains(t, text, "Vault:")
	assert.Contains(t, text, "Notes:")
	// "without --summary" hint only prints when issues exist.
	if !strings.Contains(text, "Obsidian-incompatible") && !strings.Contains(text, "Dead link") {
		assert.NotContains(t, text, "without --summary",
			"clean vault must not show the summary-mode hint")
	}
}

// --summary flag suppresses per-link details, leaving only the count
// header plus a hint about how to see the full list. Pre-2026-04-30 the
// command printed every broken link inline (174 lines on the live vault
// at one point), drowning out the summary stats. The flag is the
// "value-without-cognitive-cost" fix for the doctor firehose.
func TestDoctor_SummaryFlagSuppressesLinkDetails(t *testing.T) {
	vault := buildIndexedTestVault(t)
	full, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	summary, _, err := runRootCmd(t, "doctor", "--vault", vault, "--summary")
	require.NoError(t, err)

	// Summary has the header.
	assert.Contains(t, summary.String(), "Obsidian-incompatible links",
		"summary must still show the count header")
	// But not the per-link arrow detail that the full output has.
	assert.NotContains(t, summary.String(), "→ [[",
		"summary must NOT print per-link rewrite arrows")
	// And the hint on how to see them is present.
	assert.Contains(t, summary.String(), "without --summary",
		"summary must point the user at the full output flag")
	// Seam-3: the remedy must point at the shipped `doctor heal wikilinks`
	// command, NOT the unshipped scripts/fix_wikilinks.py helper.
	assert.Contains(t, summary.String(), "vaultmind doctor heal wikilinks",
		"summary must point the user at the shipped heal command")
	assert.NotContains(t, summary.String(), "fix_wikilinks.py",
		"summary must NOT reference the unshipped scripts/fix_wikilinks.py")
	// Sanity: the full output DID have the arrows.
	assert.Contains(t, full.String(), "→ [[",
		"full output must show per-link arrows so the test catches a regression in either direction")
}

// writeEmbeddingStatus unit-level contract: nil input is a no-op, semantic
// ready renders counts, semantic-not-ready renders the remedy line.
func TestWriteEmbeddingStatus_NilInputIsNoop(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, nil))
	assert.Empty(t, buf.String())
}

func TestWriteEmbeddingStatus_NotReadyPrintsRemedy(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: false, TotalNotes: 12,
	}))
	out := buf.String()
	assert.Contains(t, out, "Embeddings: none (12 notes)")
	assert.Contains(t, out, "vaultmind index --embed")
	assert.NotContains(t, out, "--model bge-m3",
		"the 'none' remedy must run on any backend — bge-m3 is refused on the pure-Go binary")
}

// Mixed-model state surfaces the per-model breakdown so the operator
// can see what fraction is which model. Without this branch the
// "Embeddings: ... (mixed)" line tells operators something is off but
// not what — leaving them with no path to "wait or re-embed."
func TestWriteEmbeddingStatus_MixedModelSurfacesBreakdown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true,
		TotalNotes:    100,
		Model:         "mixed",
		DenseCount:    100,
		SparseCount:   100,
		ColBERTCount:  100,
		MixedModel: []query.DoctorModelBreakdown{
			{Model: "bge-m3", Count: 60},
			{Model: "minilm", Count: 40},
		},
	}))
	out := buf.String()
	assert.Contains(t, out, "mixed-model state:")
	assert.Contains(t, out, "60 bge-m3")
	assert.Contains(t, out, "40 minilm")
}

// Modality imbalance branch — when sparse or colbert counts diverge
// from dense, the warning line names the fix command. Locks in the
// operator's path-to-remedy.
func TestWriteEmbeddingStatus_ModalityImbalanceSurfacesRemedy(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady:        true,
		TotalNotes:           100,
		Model:                "bge-m3",
		DenseCount:           100,
		SparseCount:          80,
		ColBERTCount:         70,
		HasModalityImbalance: true,
	}))
	out := buf.String()
	assert.Contains(t, out, "Partial BGE-M3 coverage")
	assert.Contains(t, out, "20 note(s) missing sparse")
	assert.Contains(t, out, "30 missing colbert")
	assert.Contains(t, out, "vaultmind index --embed --model bge-m3")
}

// MiniLM index → a first-class degraded-recall WARN naming the upgrade path.
// focalc field report P1: the MiniLM↔BGE-M3 quality cliff was silent — doctor
// printed "(minilm)" factually but offered no judgment or remedy.
func TestWriteEmbeddingStatus_MiniLMSurfacesDegradedWarning(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 50, Model: "minilm",
		DenseCount: 50, SparseCount: 0, ColBERTCount: 0,
	}))
	out := buf.String()
	assert.Contains(t, out, "degraded recall")
	assert.Contains(t, out, "MiniLM")
	assert.Contains(t, out, "embedding-backends.md")
}

// A full BGE-M3 index must NOT emit the degraded-recall warning.
func TestWriteEmbeddingStatus_BGEM3NoDegradedWarning(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 50, Model: "bge-m3",
		DenseCount: 50, SparseCount: 50, ColBERTCount: 50,
	}))
	assert.NotContains(t, buf.String(), "degraded recall")
}

func TestWriteEmbeddingStatus_ReadyPrintsCounts(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 10, DenseCount: 10, Model: "bge-m3",
		SparseCount: 8, ColBERTCount: 9,
	}))
	out := buf.String()
	assert.Contains(t, out, "dense 10/10")
	assert.Contains(t, out, "bge-m3")
	assert.Contains(t, out, "sparse 8/10")
	assert.Contains(t, out, "colbert 9/10")
}

// When the doctor flags modality imbalance, writeEmbeddingStatus must render
// the warning line AND the remediation. A silent pass would defeat the whole
// point of the field (surfacing the 2026-04-24 failure mode).
func TestWriteEmbeddingStatus_ImbalancePrintsWarning(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 24, DenseCount: 24, Model: "bge-m3",
		SparseCount: 16, ColBERTCount: 16,
		HasModalityImbalance: true,
	}))
	out := buf.String()
	assert.Contains(t, out, "Partial BGE-M3 coverage", "must name the failure mode")
	assert.Contains(t, out, "8 note(s) missing sparse", "must report the exact deficit")
	assert.Contains(t, out, "8 missing colbert", "must report the exact deficit")
	assert.Contains(t, out, "vaultmind index --embed", "must name the remedy")
}

// In a mixed-state vault, writeEmbeddingStatus must surface the per-model
// breakdown — without this line, the operator only sees "(mixed)" in the
// summary and can't tell whether the vault is mostly upgraded (5 minilm /
// 76 bge-m3) or mostly stale (47 minilm / 31 bge-m3). See vaultmind#22.
func TestWriteEmbeddingStatus_MixedStateSurfacesBreakdown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 78, DenseCount: 78, Model: "mixed",
		SparseCount: 31, ColBERTCount: 31,
		MixedModel: []query.DoctorModelBreakdown{
			{Model: "minilm", Count: 47},
			{Model: "bge-m3", Count: 31},
		},
		HasModalityImbalance: true,
	}))
	out := buf.String()
	assert.Contains(t, out, "(mixed)", "summary line must name the model as mixed")
	assert.Contains(t, out, "mixed-model state:", "breakdown line must appear")
	assert.Contains(t, out, "47 minilm", "breakdown must show MiniLM count")
	assert.Contains(t, out, "31 bge-m3", "breakdown must show BGE-M3 count")
}

// Full coverage under BGE-M3 must NOT print the warning — a false alarm would
// train users to ignore the line when it does matter.
func TestWriteEmbeddingStatus_FullCoverageSilent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 10, DenseCount: 10, Model: "bge-m3",
		SparseCount: 10, ColBERTCount: 10,
		HasModalityImbalance: false,
	}))
	assert.NotContains(t, buf.String(), "Partial BGE-M3 coverage")
}

// git status on a non-git vault must not error — users routinely initialise
// a vault before `git init`. Silent success with repo_detected=false is
// the contract.
func TestGitStatus_NonGitDirectoryReportsRepoNotDetected(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "git", "status", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			RepoDetected bool `json:"repo_detected"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.False(t, env.Result.RepoDetected)
}

// index --json on a missing vault path must return a structured error
// envelope (not crash, not a bare Go error with no code).
func TestIndex_MissingVaultReturnsStructuredError(t *testing.T) {
	out, _, err := runRootCmd(t, "index", "--vault", "/nonexistent/path", "--json")
	// WriteJSONError writes and returns nil; tolerate either outcome.
	_ = err

	if out.Len() > 0 {
		var env struct {
			Status string `json:"status"`
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "error", env.Status)
		require.NotEmpty(t, env.Errors)
		assert.Equal(t, "vault_not_found", env.Errors[0].Code)
	} else {
		require.Error(t, err)
	}
}

// Incremental index over an unchanged vault must report 0 added/updated/deleted
// — if it thought every note had changed, every re-index would nuke embeddings.
func TestIndex_IncrementalReturnsSkipsForUnchangedVault(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "index", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Index struct {
				Added   int `json:"Added"`
				Updated int `json:"Updated"`
				Deleted int `json:"Deleted"`
				Skipped int `json:"Skipped"`
			} `json:"Index"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, 0, env.Result.Index.Added, "unchanged vault must add nothing")
	assert.Equal(t, 0, env.Result.Index.Updated, "unchanged vault must update nothing")
	assert.Greater(t, env.Result.Index.Skipped, 0, "unchanged files must be skipped (mtime/hash path)")
}

// dataview lint on a vault without dataview markers finishes clean.
// Silent failure here would mask real syntax issues from users who rely
// on the clean-vault signal.
func TestDataviewLint_CleanVaultReportsValid(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			FilesChecked int   `json:"files_checked"`
			Valid        int   `json:"valid"`
			Issues       []any `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Greater(t, env.Result.FilesChecked, 0)
	assert.Empty(t, env.Result.Issues)
}

// dataview lint flags an unterminated START marker with no matching END.
// The "warning" envelope status is the script-visible signal that something
// needs fixing; losing it would let CI pipelines swallow real breakage.
func TestDataviewLint_FlagsUnterminatedStartMarker(t *testing.T) {
	vault := buildIndexedTestVault(t)
	// Real marker format: <!-- VAULTMIND:GENERATED:<key>:START --> / :END -->
	// Here we open but never close, which ValidateMarkers reports.
	bad := filepath.Join(vault, "malformed.md")
	require.NoError(t, os.WriteFile(bad, []byte(`---
id: bad-1
type: concept
title: Broken
---

<!-- VAULTMIND:GENERATED:list:START -->
content with no matching end marker
`), 0o644))

	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Issues []struct {
				Rule string `json:"rule"`
			} `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "warning", env.Status)
	assert.NotEmpty(t, env.Result.Issues, "unterminated START marker must produce at least one issue")
}

// dataview render on a target with no marker produces a structured
// "not found" style error rather than a panic.
func TestDataviewRender_MissingMarkerReturnsError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "render", "concepts/alpha.md",
		"--section-key", "nonexistent", "--vault", vault, "--json")
	// Either error Go or structured envelope; at least one signal must fire.
	if err == nil {
		assert.Contains(t, strings.ToLower(out.String()), "error")
	}
}

// retrievalModeLabel returns "hybrid" when Embedder is non-nil, "keyword"
// otherwise — this label feeds experiment telemetry and scripts branch on it.
func TestRetrievalModeLabel_HybridWhenEmbedderPresent(t *testing.T) {
	assert.Equal(t, "hybrid", retrievalModeLabel(query.AutoRetrieverResult{Embedder: stubEmbedder{}}))
	assert.Equal(t, "keyword", retrievalModeLabel(query.AutoRetrieverResult{}))
}

// ask without an argument is a usage error.
func TestAsk_MissingArgUsageError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "ask", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// ask on a small indexed vault must return a non-nil result with the query
// recorded — this is the happy-path contract the companion persona hook depends on.
func TestAsk_ReturnsResultWithQuery(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "ask", "Alpha body", "--vault", vault, "--json",
		"--budget", "2000", "--max-items", "4", "--search-limit", "5")
	require.NoError(t, err)
	// Envelope must parse and status must be "ok".
	var env struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.NotEmpty(t, env.Result)
}

// guardBGEM3SlowBackend must refuse by default when the binary would run
// BGE-M3 indexing on pure-Go hugot. Silent slow paths are the class of bug
// that cost 45 minutes on 8 notes during the 2026-04-24 investigation.
// The refusal must include a concrete pointer at `task build:ort` so the
// operator knows what to do next, not just what not to do.
func TestGuardBGEM3SlowBackend_RefusesOnGoBackend(t *testing.T) {
	if embedding.BackendName() != "go" {
		t.Skip("guard only fires on pure-Go builds")
	}
	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := guardBGEM3SlowBackend(cmd, "bge-m3")
	require.Error(t, err, "default-off allow flag must block the slow path")
	assert.Contains(t, err.Error(), "--allow-slow-backend",
		"error must name the override flag so the operator can opt in intentionally")
	assert.Contains(t, stderr.String(), "task build:ort",
		"warning must point at the supported fix path")
}

// MiniLM indexing (the other supported model) must never trigger the
// guard — the slow-path warning is specifically about BGE-M3.
func TestGuardBGEM3SlowBackend_PassesForMiniLM(t *testing.T) {
	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	require.NoError(t, guardBGEM3SlowBackend(cmd, "minilm"))
	assert.Empty(t, stderr.String(), "MiniLM must not trigger the BGE-M3 warning")
}

// ask on a zero-hit query in human mode must emit the keyword-only hint.
// (Vault has no embeddings → ask auto-picks keyword mode; an unknown query
// gives zero hits → the fallback diagnostic must fire.) Covers runAsk's
// writeZeroHitDiagnostics invocation path end-to-end.
func TestAsk_ZeroHitHumanModeEmitsKeywordHint(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "ask",
		"xyzzyverylongnonexistenttermxyzzy",
		"--vault", vault,
		"--budget", "1000", "--max-items", "2", "--search-limit", "3")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "no embeddings",
		"zero-hit ask on a vault without embeddings must name the cause")
	assert.Contains(t, text, "vaultmind index --embed",
		"zero-hit ask must point at the remedy command")
}
