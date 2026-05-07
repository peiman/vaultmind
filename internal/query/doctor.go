// Package query provides read-only vault diagnostics and search operations.
package query

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
)

// DoctorResult is the JSON-serializable output of the doctor command.
type DoctorResult struct {
	VaultPath         string            `json:"vault_path"`
	TotalFiles        int               `json:"total_files"`
	DomainNotes       int               `json:"domain_notes"`
	UnstructuredNotes int               `json:"unstructured_notes"`
	IndexStatus       string            `json:"index_status"`
	Embeddings        *DoctorEmbeddings `json:"embeddings,omitempty"`
	Issues            DoctorIssues      `json:"issues"`
}

// DoctorEmbeddings reports the vault's semantic-retrieval readiness. Surfaces
// which embedding lanes are populated so a user can diagnose a keyword-only
// fallback at a glance without running an ask query and hitting zero hits.
//
// HasModalityImbalance flags the failure mode where a BGE-M3 vault has dense
// embeddings but some notes are missing sparse or colbert. Under hybrid RRF
// this silently compresses ranking: a partially-covered note at rank 1 in 2
// lanes loses to a ubiquitous rank-3 note across 4 lanes. Dense-only vaults
// (MiniLM) are never flagged — sparse/colbert don't apply to that model.
type DoctorEmbeddings struct {
	TotalNotes           int    `json:"total_notes"`
	DenseCount           int    `json:"dense_count"`
	SparseCount          int    `json:"sparse_count"`
	ColBERTCount         int    `json:"colbert_count"`
	Model                string `json:"model"` // "bge-m3", "minilm", "mixed", or "" when no dense embeddings
	SemanticReady        bool   `json:"semantic_ready"`
	HasModalityImbalance bool   `json:"has_modality_imbalance"`
	// MixedModel is non-nil when the vault has notes embedded with more than
	// one model (e.g. mid-upgrade from MiniLM to BGE-M3). Each entry pairs a
	// model name with its row count. When set, Model == "mixed". Surfacing
	// this explicitly prevents the silent-failure shape where doctor reports
	// "bge-m3" while half the rows are still MiniLM. See vaultmind#22.
	MixedModel []DoctorModelBreakdown `json:"mixed_model,omitempty"`
}

// DoctorModelBreakdown is one entry in DoctorEmbeddings.MixedModel.
type DoctorModelBreakdown struct {
	Model string `json:"model"` // "bge-m3", "minilm", or "unknown"
	Count int    `json:"count"`
}

// DoctorIssues holds counts of vault health issues.
type DoctorIssues struct {
	DuplicateIDs              int                `json:"duplicate_ids"`
	BrokenReferences          int                `json:"broken_references"`
	MissingRequiredFields     int                `json:"missing_required_fields"`
	MalformedMarkers          int                `json:"malformed_markers"`
	UnresolvedLinks           int                `json:"unresolved_links"`
	NotesMissingIDOrType      int                `json:"notes_missing_id_or_type"`
	UnresolvedLinkDetails     []UnresolvedLink   `json:"unresolved_link_details,omitempty"`
	ObsidianIncompatibleLinks int                `json:"obsidian_incompatible_links"`
	IncompatibleLinkDetails   []IncompatibleLink `json:"incompatible_link_details,omitempty"`
	PathPseudoIDLinks         int                `json:"path_pseudo_id_links"`
	PathPseudoIDDetails       []UnresolvedLink   `json:"path_pseudo_id_details,omitempty"`

	// StaleIndex counts domain notes whose current file content hash
	// differs from the indexer's stored hash — i.e. notes edited AFTER
	// the last `vaultmind index` pass. The operator-visible signal that
	// downstream artifacts (index, embeddings, marker sections) are out
	// of sync with the source. See DetectContentDrift for the detection
	// contract.
	//
	// Replaced an earlier mtime-based detector that produced ~95% false
	// positives on real vaults — git checkouts/branch switches/pulls
	// bump mtime without touching content, and the prior signal could
	// not distinguish "edited" from "VCS-touched". Hash comparison is
	// precise: only actual content edits trigger drift.
	StaleIndex        int            `json:"stale_index"`
	StaleIndexDetails []ContentDrift `json:"stale_index_details,omitempty"`

	// HookDrift counts Claude Code hook scripts in the project's
	// `.claude/scripts/` whose bytes differ from the embedded canonical
	// in `internal/hookscripts/`. Surfaces "the foundation has rotted"
	// — copies were edited, or the binary was upgraded but old copies
	// linger. Resolution: `vaultmind hooks install --force <project>`.
	// Populated by cmd/doctor.go (project dir comes from there); query
	// layer keeps the type but doesn't import internal/hooks (business
	// layer isolation per ADR-009).
	HookDrift        int      `json:"hook_drift"`
	HookDriftDetails []string `json:"hook_drift_details,omitempty"`

	// LegacyHooksJSON is true when `.claude/hooks.json` exists at the
	// project root. That standalone file is no longer recognized by
	// Claude Code 2.1.129+ — projects with it have silently broken
	// hooks. The fix is to migrate the contents into
	// `.claude/settings.json` under a top-level `hooks` key.
	// Live evidence from workhorse dogfood 2026-05-06/07.
	// Populated by cmd/doctor.go (project dir comes from there); query
	// layer keeps the type but doesn't import internal/hooks per
	// ADR-009 business-business isolation.
	LegacyHooksJSON bool `json:"legacy_hooks_json"`
}

// ContentDrift describes one note whose current file content hash
// differs from the indexer's stored hash.
type ContentDrift struct {
	NoteID      string `json:"note_id"`
	Path        string `json:"path"`
	CurrentHash string `json:"current_hash"` // sha256(file content)
	StoredHash  string `json:"stored_hash"`  // notes.hash from index DB
}

// UnresolvedLink describes a single unresolved link with source and target info.
type UnresolvedLink struct {
	SourceID   string `json:"source_id"`
	SourcePath string `json:"source_path"`
	TargetRaw  string `json:"target_raw"`
}

// IncompatibleLink describes a wikilink that resolves in VaultMind but not in Obsidian.
// Obsidian resolves [[target]] by matching target against filenames (without extension).
// If dst_raw uses a note's title instead of its filename stem, Obsidian won't find it.
type IncompatibleLink struct {
	SourceID     string `json:"source_id"`
	SourcePath   string `json:"source_path"`
	TargetRaw    string `json:"target_raw"`
	SuggestedFix string `json:"suggested_fix"`
}

// Doctor runs vault health diagnostics against the indexed database.
//
// When reg is non-nil, the missing_required_fields counter is populated
// by running the schema validator and summing missing-field issues. When
// reg is nil, the counter stays 0 (caller hasn't loaded a registry —
// usually a misconfiguration). The signature was extended 2026-05-04 to
// close the silent-failure shape where MissingRequiredFields was a
// declared output that never got populated.
func Doctor(db *index.DB, vaultPath string, reg *schema.Registry) (*DoctorResult, error) {
	result := &DoctorResult{
		VaultPath:   vaultPath,
		IndexStatus: "current",
	}

	// Count total, domain, unstructured
	if err := db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&result.TotalFiles); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE is_domain = TRUE").Scan(&result.DomainNotes); err != nil {
		return nil, fmt.Errorf("counting domain notes: %w", err)
	}
	result.UnstructuredNotes = result.TotalFiles - result.DomainNotes

	// missing_required_fields — call Validate and count. Single source
	// of truth for what counts as "missing required field"; doctor
	// surfaces the aggregate, validate surfaces per-note details.
	if reg != nil {
		validateRes, err := Validate(db, reg)
		if err != nil {
			return nil, fmt.Errorf("validating for missing-required-fields: %w", err)
		}
		for _, issue := range validateRes.Issues {
			if issue.Rule == "missing_required_field" {
				result.Issues.MissingRequiredFields++
			}
		}
	}

	// Unresolved links
	if err := db.QueryRow("SELECT COUNT(*) FROM links WHERE resolved = FALSE AND dst_note_id IS NULL").Scan(&result.Issues.UnresolvedLinks); err != nil {
		return nil, fmt.Errorf("counting unresolved links: %w", err)
	}

	// Unresolved link details
	if result.Issues.UnresolvedLinks > 0 {
		detailRows, err := db.Query(
			`SELECT l.src_note_id, COALESCE(n.path, ''), l.dst_raw
			 FROM links l
			 LEFT JOIN notes n ON n.id = l.src_note_id
			 WHERE l.resolved = FALSE AND l.dst_note_id IS NULL`)
		if err != nil {
			return nil, fmt.Errorf("querying unresolved link details: %w", err)
		}
		defer func() { _ = detailRows.Close() }()

		result.Issues.UnresolvedLinkDetails = []UnresolvedLink{}
		for detailRows.Next() {
			var ul UnresolvedLink
			if scanErr := detailRows.Scan(&ul.SourceID, &ul.SourcePath, &ul.TargetRaw); scanErr != nil {
				return nil, fmt.Errorf("scanning unresolved link detail: %w", scanErr)
			}
			result.Issues.UnresolvedLinkDetails = append(result.Issues.UnresolvedLinkDetails, ul)
		}
		if err := detailRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating unresolved link details: %w", err)
		}
	}

	// Duplicate IDs (should be 0 with UNIQUE constraint, but check anyway)
	if err := db.QueryRow("SELECT COUNT(*) FROM (SELECT id FROM notes GROUP BY id HAVING COUNT(*) > 1)").Scan(&result.Issues.DuplicateIDs); err != nil {
		return nil, fmt.Errorf("counting duplicate IDs: %w", err)
	}

	// Links resolved to _path: pseudo-IDs (files that don't exist in the vault)
	pseudoRows, err := db.Query(`
		SELECT l.src_note_id, COALESCE(sn.path, ''), l.dst_raw
		FROM links l
		LEFT JOIN notes sn ON sn.id = l.src_note_id
		WHERE l.resolved = TRUE AND SUBSTR(l.dst_note_id, 1, 6) = '_path:'`)
	if err != nil {
		return nil, fmt.Errorf("querying pseudo-ID links: %w", err)
	}
	defer func() { _ = pseudoRows.Close() }()

	result.Issues.PathPseudoIDDetails = []UnresolvedLink{}
	for pseudoRows.Next() {
		var pl UnresolvedLink
		if scanErr := pseudoRows.Scan(&pl.SourceID, &pl.SourcePath, &pl.TargetRaw); scanErr != nil {
			return nil, fmt.Errorf("scanning pseudo-ID link: %w", scanErr)
		}
		result.Issues.PathPseudoIDDetails = append(result.Issues.PathPseudoIDDetails, pl)
	}
	if err := pseudoRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pseudo-ID links: %w", err)
	}
	result.Issues.PathPseudoIDLinks = len(result.Issues.PathPseudoIDDetails)

	// Obsidian-incompatible links: resolved wikilinks where dst_raw doesn't
	// match the target note's filename stem. Obsidian resolves [[X]] by matching
	// X against filenames (without extension and directory), not note titles.
	incompatRows, err := db.Query(`
		SELECT l.src_note_id, COALESCE(sn.path, ''), l.dst_raw, n.path
		FROM links l
		JOIN notes n ON n.id = l.dst_note_id
		LEFT JOIN notes sn ON sn.id = l.src_note_id
		WHERE l.resolved = TRUE
		AND l.edge_type IN ('explicit_link', 'explicit_embed')`)
	if err != nil {
		return nil, fmt.Errorf("querying incompatible links: %w", err)
	}
	defer func() { _ = incompatRows.Close() }()

	result.Issues.IncompatibleLinkDetails = []IncompatibleLink{}
	seen := make(map[string]bool)
	for incompatRows.Next() {
		var srcID, srcPath, dstRaw, dstPath string
		if scanErr := incompatRows.Scan(&srcID, &srcPath, &dstRaw, &dstPath); scanErr != nil {
			return nil, fmt.Errorf("scanning incompatible link: %w", scanErr)
		}
		filenameStem := strings.TrimSuffix(filepath.Base(dstPath), ".md")
		if dstRaw == filenameStem {
			continue // already compatible
		}
		// Also skip if dst_raw contains "|" (already uses [[file|display]] format)
		if strings.Contains(dstRaw, "|") {
			continue
		}
		key := srcID + "\x00" + dstRaw
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Issues.IncompatibleLinkDetails = append(result.Issues.IncompatibleLinkDetails, IncompatibleLink{
			SourceID:     srcID,
			SourcePath:   srcPath,
			TargetRaw:    dstRaw,
			SuggestedFix: filenameStem,
		})
	}
	if err := incompatRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating incompatible links: %w", err)
	}
	result.Issues.ObsidianIncompatibleLinks = len(result.Issues.IncompatibleLinkDetails)

	emb, err := collectEmbeddingStatus(db, result.TotalFiles)
	if err != nil {
		return nil, err
	}
	result.Embeddings = emb

	// Drift detector: current file content hash vs stored DB hash.
	// Surfaces "content edited since last index" so operators know when
	// to re-run `vaultmind index`. Hash comparison is precise — git
	// operations that bump mtime without changing content do NOT trigger
	// drift (the false-positive shape that retired the prior mtime-based
	// detector).
	drifts, err := DetectContentDrift(db, vaultPath)
	if err != nil {
		return nil, fmt.Errorf("detecting content drift: %w", err)
	}
	result.Issues.StaleIndex = len(drifts)
	if len(drifts) > 0 {
		result.Issues.StaleIndexDetails = drifts
	}

	return result, nil
}

// collectEmbeddingStatus inspects the index DB for per-lane embedding counts
// and infers the model from dense-embedding dimensionality (384=minilm,
// 1024=bge-m3). SemanticReady is driven by dense presence since it's the
// required lane for ask's auto-retriever to engage hybrid mode.
func collectEmbeddingStatus(db *index.DB, total int) (*DoctorEmbeddings, error) {
	emb := &DoctorEmbeddings{TotalNotes: total}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL").Scan(&emb.DenseCount); err != nil {
		return nil, fmt.Errorf("counting dense embeddings: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE sparse_embedding IS NOT NULL").Scan(&emb.SparseCount); err != nil {
		return nil, fmt.Errorf("counting sparse embeddings: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE colbert_embedding IS NOT NULL").Scan(&emb.ColBERTCount); err != nil {
		return nil, fmt.Errorf("counting colbert embeddings: %w", err)
	}
	emb.SemanticReady = emb.DenseCount > 0
	if emb.DenseCount > 0 {
		// Use the per-dims breakdown so mixed-state vaults (e.g. partial
		// upgrade from MiniLM to BGE-M3) are surfaced explicitly rather than
		// classified by whichever row SQLite scans first. vaultmind#22.
		counts, err := index.DetectEmbeddingDimsCounts(db)
		if err != nil {
			return nil, fmt.Errorf("counting embedding dims: %w", err)
		}
		switch len(counts) {
		case 0:
			// No dense rows — leave Model empty.
		case 1:
			emb.Model = modelNameForDims(counts[0].Dims)
		default:
			emb.Model = "mixed"
			emb.MixedModel = make([]DoctorModelBreakdown, 0, len(counts))
			for _, c := range counts {
				emb.MixedModel = append(emb.MixedModel, DoctorModelBreakdown{
					Model: modelNameForDims(c.Dims),
					Count: c.Count,
				})
			}
		}
	}
	// Modality imbalance only makes sense when BGE-M3 is involved — sparse
	// and colbert lanes are populated in lockstep with BGE-M3 dense. Mixed
	// vaults flag imbalance because the BGE-M3 fraction must be in lockstep
	// for hybrid RRF to work; the MiniLM fraction has no sparse/colbert by
	// design but is also not delivering hybrid retrieval anyway, which is
	// the operator-visible problem.
	if (emb.Model == "bge-m3" || emb.Model == "mixed") &&
		(emb.SparseCount < emb.DenseCount || emb.ColBERTCount < emb.DenseCount) {
		emb.HasModalityImbalance = true
	}
	return emb, nil
}

// DetectContentDrift compares each domain note's current file
// content hash against the indexer's stored hash and returns the
// notes whose content has changed since the last `vaultmind index`
// pass.
//
// Hash-based, NOT mtime-based: git checkouts, branch switches, and
// other VCS operations bump file mtime without touching content. The
// prior mtime-based detector produced ~95% false positives on real
// vaults (385 of 407 notes after a routine `git checkout main`).
// sha256 over the full file gives precise content identity — only
// real content edits trigger drift.
//
// Per-note IO failures (deleted files, unreadable permissions) are
// silently skipped. The indexer reports those via its own path; doctor's
// job is health summary, not filesystem-error reporting. ORDER BY path
// gives deterministic output (the experiment framework consumes this
// JSON; stable order avoids spurious diffs).
func DetectContentDrift(db *index.DB, vaultPath string) ([]ContentDrift, error) {
	rows, err := db.Query(`SELECT id, path, hash FROM notes WHERE is_domain = TRUE ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("querying domain notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	drifts := make([]ContentDrift, 0)
	for rows.Next() {
		// Plain strings: schema declares id/path/hash as NOT NULL
		// (001_baseline_schema.sql), so NullString defensiveness here
		// would be dead code. If a future migration makes any nullable,
		// add a typed guard with an explicit test for that path.
		var id, path, storedHash string
		if scanErr := rows.Scan(&id, &path, &storedHash); scanErr != nil {
			return nil, fmt.Errorf("scanning domain note: %w", scanErr)
		}
		abs := filepath.Join(vaultPath, path)
		// abs is vault root + DB-stored relative path, not raw user input.
		content, readErr := os.ReadFile(abs) // #nosec G304
		if readErr != nil {
			continue
		}
		h := sha256.Sum256(content)
		currentHash := fmt.Sprintf("%x", h[:])
		if currentHash != storedHash {
			drifts = append(drifts, ContentDrift{
				NoteID:      id,
				Path:        path,
				CurrentHash: currentHash,
				StoredHash:  storedHash,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return drifts, nil
}

// modelNameForDims maps a dense embedding length (in float32 elements) to
// the canonical model name. Centralised so doctor and any future caller
// agree.
func modelNameForDims(dims int) string {
	switch dims {
	case 384:
		return "minilm"
	case 1024:
		return "bge-m3"
	default:
		return "unknown"
	}
}
