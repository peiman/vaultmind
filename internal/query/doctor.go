// Package query provides read-only vault diagnostics and search operations.
package query

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
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

	// StaleVMUpdated counts domain notes whose file mtime is later than
	// their vm_updated frontmatter timestamp — i.e. notes edited AFTER
	// vaultmind last processed them. The operator-visible signal that
	// downstream artifacts (index, embeddings) may be stale relative to
	// source. Missing-vm_updated notes are NOT counted here; that's
	// frontmatter fix's territory. See DetectVMUpdatedDrift for the
	// detection contract and tolerance window.
	StaleVMUpdated        int              `json:"stale_vm_updated"`
	StaleVMUpdatedDetails []StaleVMUpdated `json:"stale_vm_updated_details,omitempty"`
}

// StaleVMUpdated describes one note whose file mtime is later than its
// vm_updated frontmatter timestamp.
type StaleVMUpdated struct {
	NoteID    string `json:"note_id"`
	Path      string `json:"path"`
	Mtime     string `json:"mtime"`      // RFC3339-second UTC
	VMUpdated string `json:"vm_updated"` // raw value from frontmatter
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

	// Drift detector: mtime > vm_updated. Surfaces "edited since
	// vaultmind processed" so operators know when to reindex / re-embed
	// / re-mark. Reads the filesystem (not stale DB state) so the
	// signal reflects current truth.
	domainPaths, err := collectDomainNotePaths(db)
	if err != nil {
		return nil, err
	}
	drifts, err := DetectVMUpdatedDrift(vaultPath, domainPaths)
	if err != nil {
		return nil, fmt.Errorf("detecting vm_updated drift: %w", err)
	}
	result.Issues.StaleVMUpdated = len(drifts)
	if len(drifts) > 0 {
		result.Issues.StaleVMUpdatedDetails = drifts
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

// vmUpdatedDriftTolerance is the max mtime-vs-vm_updated gap that's
// NOT counted as drift. vm_updated is second-precision; the file write
// that sets it completes a fraction of a second later, so mtime is
// naturally epsilon-ahead immediately after a vaultmind write. 5s
// covers that gap with margin while still catching genuine human
// edits (always many seconds, usually minutes/hours/days, off).
const vmUpdatedDriftTolerance = 5 * time.Second

// DetectVMUpdatedDrift compares each note's file mtime against its
// vm_updated frontmatter timestamp and returns the notes whose file
// has been edited since vaultmind last processed it.
//
// Reads the filesystem directly (not the index DB) so the comparison
// reflects current reality, not last-index-pass state. The DB is the
// source of paths-to-check; the filesystem is the source of truth for
// mtime AND vm_updated.
//
// Notes with absent vm_updated are NOT counted — that's frontmatter
// fix's territory (the four-tier taxonomy says vaultmind owns the
// field; if it's absent, the contract is "vaultmind hasn't touched
// this file yet," not "drift since last touch").
//
// Notes with unparseable vm_updated ARE counted: vaultmind writes the
// field in a known format (schema.VMUpdatedFormat), so a value that
// fails to parse means corruption, and the operator should run
// `frontmatter fix --apply` to reset it. Surfacing it in drift makes
// the resolution path obvious (same fix command resolves both stale
// and corrupt cases).
//
// paths are vault-relative. Per-note IO failures are silently skipped
// — the indexer would have already reported notes that don't read.
// Doctor's job is health summary, not filesystem-error reporting.
func DetectVMUpdatedDrift(vaultPath string, paths []string) ([]StaleVMUpdated, error) {
	drifts := make([]StaleVMUpdated, 0)
	for _, rel := range paths {
		abs := filepath.Join(vaultPath, rel)
		info, statErr := os.Stat(abs)
		if statErr != nil {
			continue
		}
		// abs is constructed from vault root + DB-stored relative path,
		// not raw user input. Same trust tier as the indexer's reads.
		content, readErr := os.ReadFile(abs) // #nosec G304
		if readErr != nil {
			continue
		}
		fm, _, parseErr := parser.ExtractFrontmatter(content)
		if parseErr != nil || fm == nil {
			continue
		}
		// yaml.v3 unmarshals an unquoted RFC3339 scalar as time.Time, but
		// a quoted one as string. Vaultmind always writes quoted (the
		// canonical SSOT format contains a colon, which yaml.v3 auto-
		// quotes), but human/agent edits may produce either form. Accept
		// both so the detector doesn't silently miss drift on hand-edited
		// files. The bare-bool/int paths intentionally fall through to
		// "absent" — those values can't be a real timestamp.
		var rawVMU string
		switch v := fm["vm_updated"].(type) {
		case string:
			rawVMU = v
		case time.Time:
			rawVMU = v.UTC().Format(schema.VMUpdatedFormat)
		}
		if rawVMU == "" {
			// Absent — fix's signal, not drift's.
			continue
		}
		mtime := info.ModTime().UTC().Truncate(time.Second)
		mtimeStr := mtime.Format(schema.VMUpdatedFormat)
		noteID, _ := fm["id"].(string)
		drift := StaleVMUpdated{
			NoteID:    noteID,
			Path:      rel,
			Mtime:     mtimeStr,
			VMUpdated: rawVMU,
		}
		vmuTime, parseTimeErr := time.Parse(schema.VMUpdatedFormat, rawVMU)
		if parseTimeErr != nil {
			drifts = append(drifts, drift)
			continue
		}
		if mtime.Sub(vmuTime) > vmUpdatedDriftTolerance {
			drifts = append(drifts, drift)
		}
	}
	return drifts, nil
}

// collectDomainNotePaths returns vault-relative paths for all domain
// notes — the population the drift detector inspects. Sorted for
// deterministic output (doctor's JSON envelope is consumed by the
// experiment framework; stable order avoids spurious diffs).
func collectDomainNotePaths(db *index.DB) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM notes WHERE is_domain = TRUE ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("querying domain note paths: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var paths []string
	for rows.Next() {
		var p sql.NullString
		if scanErr := rows.Scan(&p); scanErr != nil {
			return nil, fmt.Errorf("scanning domain note path: %w", scanErr)
		}
		if p.Valid && p.String != "" {
			paths = append(paths, p.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return paths, nil
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
