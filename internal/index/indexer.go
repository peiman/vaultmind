package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
)

// IndexResult holds the outcome of an index rebuild.
type IndexResult struct {
	DBPath            string `json:"db_path"`
	Indexed           int    `json:"indexed"`
	DomainNotes       int    `json:"domain_notes"`
	UnstructuredNotes int    `json:"unstructured_notes"`
	Errors            int    `json:"errors"`
	Skipped           int    `json:"skipped"`
	DuplicateIDs      int    `json:"duplicate_ids"`
	DurationMs        int64  `json:"duration_ms"`
	CompletedAt       string `json:"completed_at"`
}

// Indexer orchestrates vault scanning, parsing, and SQLite storage.
type Indexer struct {
	vaultRoot string
	dbPath    string
	cfg       *vault.Config
}

// NewIndexer creates an Indexer for the given vault.
func NewIndexer(vaultRoot, dbPath string, cfg *vault.Config) *Indexer {
	return &Indexer{
		vaultRoot: vaultRoot,
		dbPath:    dbPath,
		cfg:       cfg,
	}
}

// Rebuild performs a full rebuild: scan all .md files, parse, and store.
func (idx *Indexer) Rebuild() (*IndexResult, error) {
	start := time.Now()

	db, err := Open(idx.dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening index database: %w", err)
	}
	defer func() { _ = db.Close() }()

	files, err := vault.Scan(idx.vaultRoot, idx.cfg.Vault.Exclude)
	if err != nil {
		return nil, fmt.Errorf("scanning vault: %w", err)
	}

	result := &IndexResult{DBPath: idx.dbPath}
	seenIDs := make(map[string]string) // id → first path

	for _, file := range files {
		content, readErr := os.ReadFile(file.AbsPath)
		if readErr != nil {
			log.Debug().Err(readErr).Str("path", file.RelPath).Msg("skipping unreadable file")
			result.Errors++
			continue
		}

		parsed, parseErr := parser.Parse(content)
		if parseErr != nil {
			log.Debug().Err(parseErr).Str("path", file.RelPath).Msg("skipping unparseable file")
			result.Errors++
			continue
		}

		rec := buildNoteRecord(file, content, parsed)

		// Check for duplicate IDs
		if rec.IsDomain {
			if firstPath, seen := seenIDs[rec.ID]; seen {
				log.Warn().
					Str("id", rec.ID).
					Str("path", file.RelPath).
					Str("first_path", firstPath).
					Msg("duplicate ID detected — second file overwrites first")
				result.DuplicateIDs++
			}
			seenIDs[rec.ID] = file.RelPath
		}

		if storeErr := StoreNote(db, rec); storeErr != nil {
			log.Debug().Err(storeErr).Str("path", file.RelPath).Msg("skipping file with store error")
			result.Errors++
			continue
		}

		result.Indexed++
		if rec.IsDomain {
			result.DomainNotes++
		} else {
			result.UnstructuredNotes++
		}
	}

	// Post-index: resolve body wikilinks against the notes table
	resolved, resolveErr := ResolveLinks(db)
	if resolveErr != nil {
		log.Debug().Err(resolveErr).Msg("link resolution pass failed")
	} else {
		log.Debug().Int("resolved", resolved).Msg("link resolution complete")
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	return result, nil
}

// ResolveLinks updates unresolved links by matching dst_raw against note IDs,
// titles, and aliases. Sets dst_note_id and resolved=TRUE for matches.
func ResolveLinks(db *DB) (int, error) {
	// Resolve by exact ID match
	res1, err := db.Exec(`
		UPDATE links SET dst_note_id = dst_raw, resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL
		AND dst_raw IN (SELECT id FROM notes)`)
	if err != nil {
		return 0, fmt.Errorf("resolving by ID: %w", err)
	}
	count1, _ := res1.RowsAffected()

	// Resolve by exact title match
	res2, err := db.Exec(`
		UPDATE links SET dst_note_id = (
			SELECT id FROM notes WHERE title = links.dst_raw LIMIT 1
		), resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL
		AND dst_raw IN (SELECT title FROM notes)`)
	if err != nil {
		return int(count1), fmt.Errorf("resolving by title: %w", err)
	}
	count2, _ := res2.RowsAffected()

	// Resolve by alias match
	res3, err := db.Exec(`
		UPDATE links SET dst_note_id = (
			SELECT note_id FROM aliases WHERE alias = links.dst_raw LIMIT 1
		), resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL
		AND dst_raw IN (SELECT alias FROM aliases)`)
	if err != nil {
		return int(count1 + count2), fmt.Errorf("resolving by alias: %w", err)
	}
	count3, _ := res3.RowsAffected()

	return int(count1 + count2 + count3), nil
}

// parserLinkTypeToEdgeType maps parser link types to SRS-05 edge type names.
func parserLinkTypeToEdgeType(lt parser.LinkType) string {
	switch lt {
	case parser.LinkTypeWikilink, parser.LinkTypeMarkdown:
		return "explicit_link"
	case parser.LinkTypeEmbed:
		return "explicit_embed"
	default:
		return string(lt)
	}
}

// buildNoteRecord transforms parser output + file info into a NoteRecord.
func buildNoteRecord(file vault.ScannedFile, content []byte, parsed *parser.ParsedNote) NoteRecord {
	h := sha256.Sum256(content)

	rec := NoteRecord{
		Path:     file.RelPath,
		Hash:     fmt.Sprintf("%x", h[:]),
		MTime:    file.ModTime.Unix(),
		IsDomain: parsed.IsDomain,
		BodyText: parsed.FTSBody,
	}

	if parsed.IsDomain {
		rec.ID = parsed.ID
	} else {
		rec.ID = "_path:" + file.RelPath
	}

	// Extract typed fields from frontmatter map
	if parsed.Frontmatter != nil {
		rec.Title = fmString(parsed.Frontmatter, "title")
		rec.Type = fmString(parsed.Frontmatter, "type")
		rec.Status = fmString(parsed.Frontmatter, "status")
		rec.Created = fmString(parsed.Frontmatter, "created")
		rec.Updated = fmString(parsed.Frontmatter, "updated")
		rec.Aliases = fmStringList(parsed.Frontmatter, "aliases")
		rec.Tags = fmStringList(parsed.Frontmatter, "tags")

		// Collect extra KV (fields not in dedicated columns)
		knownKeys := map[string]bool{
			"id": true, "type": true, "title": true, "status": true,
			"created": true, "updated": true, "vm_updated": true,
			"aliases": true, "tags": true, "parent_id": true,
			"related_ids": true, "source_ids": true,
		}
		extra := make(map[string]interface{})
		for k, v := range parsed.Frontmatter {
			if !knownKeys[k] {
				extra[k] = v
			}
		}
		if len(extra) > 0 {
			rec.ExtraKV = extra
		}
	}

	// Convert parser links to LinkRecords using SRS edge type names
	for _, link := range parsed.Links {
		rec.Links = append(rec.Links, LinkRecord{
			DstRaw:     link.Target,
			EdgeType:   parserLinkTypeToEdgeType(link.LinkType),
			TargetKind: string(link.TargetKind),
			Heading:    link.Heading,
			BlockID:    link.BlockID,
			Confidence: "high",
			Origin:     fmt.Sprintf("body:line:%d", link.Line),
		})
	}

	// Also extract frontmatter relation fields as explicit_relation edges
	if parsed.Frontmatter != nil {
		rec.Links = append(rec.Links, fmRelationLinks(parsed.Frontmatter, "parent_id", rec.ID)...)
		rec.Links = append(rec.Links, fmRelationLinks(parsed.Frontmatter, "related_ids", rec.ID)...)
		rec.Links = append(rec.Links, fmRelationLinks(parsed.Frontmatter, "source_ids", rec.ID)...)
	}

	// Convert parser headings/blocks
	for _, h := range parsed.Headings {
		rec.Headings = append(rec.Headings, HeadingRecord{
			Slug: h.Slug, Level: h.Level, Title: h.Title,
		})
	}
	for _, b := range parsed.Blocks {
		rec.Blocks = append(rec.Blocks, BlockRecord{
			BlockID: b.BlockID, Heading: b.Heading, StartLine: b.Line,
		})
	}

	return rec
}

// fmRelationLinks extracts stable-ID references from frontmatter fields
// (parent_id, related_ids, source_ids) as explicit_relation edges.
func fmRelationLinks(fm map[string]interface{}, key, srcID string) []LinkRecord {
	raw, ok := fm[key]
	if !ok {
		return nil
	}

	var ids []string
	switch v := raw.(type) {
	case string:
		if v != "" {
			ids = []string{v}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
	}

	var links []LinkRecord
	for _, id := range ids {
		links = append(links, LinkRecord{
			DstRaw:     id,
			EdgeType:   "explicit_relation",
			Confidence: "high",
			Origin:     "frontmatter." + key,
		})
	}
	return links
}

func fmString(fm map[string]interface{}, key string) string {
	v, ok := fm[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case time.Time:
		if val.Hour() == 0 && val.Minute() == 0 && val.Second() == 0 {
			return val.Format("2006-01-02")
		}
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func fmStringList(fm map[string]interface{}, key string) []string {
	v, ok := fm[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}
