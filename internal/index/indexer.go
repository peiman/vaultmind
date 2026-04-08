package index

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/internal/embedding"
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
	Added             int    `json:"added"`
	Updated           int    `json:"updated"`
	Deleted           int    `json:"deleted"`
	FullRebuild       bool   `json:"full_rebuild"`
	DurationMs        int64  `json:"duration_ms"`
	CompletedAt       string `json:"completed_at"`
}

// EmbedResult holds the outcome of an embedding pass.
type EmbedResult struct {
	Embedded int `json:"embedded"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// IndexAndEmbedResult combines index and optional embed results for command output.
type IndexAndEmbedResult struct {
	Index *IndexResult `json:"index"`
	Embed *EmbedResult `json:"embed,omitempty"`
}

// RunEmbed creates an embedder for the given model, runs EmbedNotes, and cleans up.
func (idx *Indexer) RunEmbed(ctx context.Context, dbPath, model string) (*EmbedResult, error) {
	var embedder embedding.Embedder
	var err error
	switch model {
	case "bge-m3":
		embedder, err = embedding.NewBGEM3Embedder(embedding.BGEM3Config())
	default:
		embedder, err = embedding.NewHugotEmbedder(embedding.DefaultHugotConfig())
	}
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	defer func() { _ = embedder.Close() }()

	return idx.EmbedNotes(ctx, dbPath, embedder)
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

	// Parse all files first, then store in a single batch transaction
	type parsedFile struct {
		rec  NoteRecord
		path string
	}
	var records []parsedFile

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
					Msg("duplicate ID detected — second file skipped")
				result.DuplicateIDs++
				continue // skip the duplicate — first file wins
			}
			seenIDs[rec.ID] = file.RelPath
		}

		records = append(records, parsedFile{rec: rec, path: file.RelPath})
	}

	// Batch store in a single transaction for performance
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, pf := range records {
		if storeErr := StoreNoteInTx(tx, pf.rec); storeErr != nil {
			log.Debug().Err(storeErr).Str("path", pf.path).Msg("skipping file with store error")
			result.Errors++
			continue
		}

		result.Indexed++
		if pf.rec.IsDomain {
			result.DomainNotes++
		} else {
			result.UnstructuredNotes++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing batch transaction: %w", err)
	}

	// Post-index: resolve body wikilinks against the notes table
	resolved, resolveErr := ResolveLinks(db)
	if resolveErr != nil {
		log.Debug().Err(resolveErr).Msg("link resolution pass failed")
	} else {
		log.Debug().Int("resolved", resolved).Msg("link resolution complete")
	}

	// Compute inferred edges
	aliasCount, aliasErr := ComputeAliasMentions(db, idx.cfg.Memory.AliasMinLength)
	if aliasErr != nil {
		log.Debug().Err(aliasErr).Msg("alias mention detection failed")
	} else {
		log.Debug().Int("edges", aliasCount).Msg("alias mention detection complete")
	}

	tagCount, tagErr := ComputeTagOverlap(db, idx.cfg.Memory.TagOverlapThreshold)
	if tagErr != nil {
		log.Debug().Err(tagErr).Msg("tag overlap detection failed")
	} else {
		log.Debug().Int("edges", tagCount).Msg("tag overlap detection complete")
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	return result, nil
}

// IndexFile re-indexes a single file by its vault-relative path.
func (idx *Indexer) IndexFile(relPath string) error {
	absPath := filepath.Join(idx.vaultRoot, relPath)

	content, err := os.ReadFile(absPath) //nolint:gosec // absPath is built from vaultRoot (trusted) + relPath (vault-relative)
	if err != nil {
		return fmt.Errorf("reading file %q: %w", relPath, err)
	}

	parsed, err := parser.Parse(content)
	if err != nil {
		return fmt.Errorf("parsing file %q: %w", relPath, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat file %q: %w", relPath, err)
	}

	file := vault.ScannedFile{
		RelPath: relPath,
		AbsPath: absPath,
		ModTime: info.ModTime(),
	}

	rec := buildNoteRecord(file, content, parsed)

	db, err := Open(idx.dbPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer func() { _ = db.Close() }()

	return StoreNote(db, rec)
}

// Incremental scans the vault and only indexes files that are new or changed
// (detected via content hash). Deleted files are removed from the index.
func (idx *Indexer) Incremental() (*IndexResult, error) {
	start := time.Now()

	db, err := Open(idx.dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening index database: %w", err)
	}
	defer func() { _ = db.Close() }()

	stored, err := db.NoteHashes()
	if err != nil {
		return nil, fmt.Errorf("loading stored hashes: %w", err)
	}

	files, err := vault.Scan(idx.vaultRoot, idx.cfg.Vault.Exclude)
	if err != nil {
		return nil, fmt.Errorf("scanning vault: %w", err)
	}

	result := &IndexResult{DBPath: idx.dbPath}
	diskPaths := make(map[string]bool, len(files))

	for _, file := range files {
		diskPaths[file.RelPath] = true

		info, ok := stored[file.RelPath]

		// Mtime fast path: skip file read entirely if mtime unchanged
		if ok && file.ModTime.Unix() == info.MTime {
			result.Skipped++
			continue
		}

		// Mtime changed or new file — must read and hash
		content, readErr := os.ReadFile(file.AbsPath) //nolint:gosec // vault path is trusted
		if readErr != nil {
			log.Debug().Err(readErr).Str("path", file.RelPath).Msg("skipping unreadable file")
			result.Errors++
			continue
		}

		h := sha256.Sum256(content)
		hash := fmt.Sprintf("%x", h[:])

		// Hash unchanged — just update mtime, skip parse
		if ok && hash == info.Hash {
			if mtErr := db.UpdateMTime(file.RelPath, file.ModTime.Unix()); mtErr != nil {
				log.Debug().Err(mtErr).Str("path", file.RelPath).Msg("failed to update mtime")
			}
			result.Skipped++
			continue
		}

		parsed, parseErr := parser.Parse(content)
		if parseErr != nil {
			log.Debug().Err(parseErr).Str("path", file.RelPath).Msg("skipping unparseable file")
			result.Errors++
			continue
		}

		rec := buildNoteRecord(file, content, parsed)
		if storeErr := StoreNote(db, rec); storeErr != nil {
			log.Debug().Err(storeErr).Str("path", file.RelPath).Msg("skipping file with store error")
			result.Errors++
			continue
		}

		if ok {
			result.Updated++
		} else {
			result.Added++
		}
		result.Indexed++
	}

	for storedPath := range stored {
		if !diskPaths[storedPath] {
			if delErr := DeleteNoteByPath(db, storedPath); delErr != nil {
				log.Debug().Err(delErr).Str("path", storedPath).Msg("failed to delete orphaned note")
				result.Errors++
				continue
			}
			result.Deleted++
		}
	}

	resolved, resolveErr := ResolveLinks(db)
	if resolveErr != nil {
		log.Debug().Err(resolveErr).Msg("link resolution failed")
	} else {
		log.Debug().Int("resolved", resolved).Msg("link resolution complete")
	}

	// Compute inferred edges
	aliasCount, aliasErr := ComputeAliasMentions(db, idx.cfg.Memory.AliasMinLength)
	if aliasErr != nil {
		log.Debug().Err(aliasErr).Msg("alias mention detection failed")
	} else {
		log.Debug().Int("edges", aliasCount).Msg("alias mention detection complete")
	}

	tagCount, tagErr := ComputeTagOverlap(db, idx.cfg.Memory.TagOverlapThreshold)
	if tagErr != nil {
		log.Debug().Err(tagErr).Msg("tag overlap detection failed")
	} else {
		log.Debug().Int("edges", tagCount).Msg("tag overlap detection complete")
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	return result, nil
}

// EmbedNotes computes and stores embeddings for all notes that don't have one yet.
// It opens its own DB connection (like Rebuild/Incremental) so it can be called
// after the indexer has closed its connection.
func (idx *Indexer) EmbedNotes(ctx context.Context, dbPath string, embedder embedding.Embedder) (*EmbedResult, error) {
	db, err := Open(dbPath) //nolint:contextcheck // Open does not accept a context; ctx is for the embedder calls
	if err != nil {
		return nil, fmt.Errorf("opening index database for embedding: %w", err)
	}
	defer func() { _ = db.Close() }()

	result := &EmbedResult{}

	// Determine skip/pending query based on embedder type.
	// For FullEmbedder (BGE-M3): a note needs embedding if ANY of the three columns is NULL.
	// For dense-only (MiniLM): a note needs embedding if the dense column is NULL.
	fullEmbedder, isFull := embedder.(embedding.FullEmbedder)

	var skipQuery, pendingQuery string
	if isFull {
		// BGE-M3: need notes where any of the 3 embeddings are missing
		pendingQuery = "SELECT id, body_text FROM notes WHERE embedding IS NULL OR sparse_embedding IS NULL OR colbert_embedding IS NULL"
		skipQuery = "SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL AND sparse_embedding IS NOT NULL AND colbert_embedding IS NOT NULL"
	} else {
		pendingQuery = "SELECT id, body_text FROM notes WHERE embedding IS NULL"
		skipQuery = "SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL"
	}

	var skippedCount int
	err = db.QueryRow(skipQuery).Scan(&skippedCount)
	if err != nil {
		return nil, fmt.Errorf("counting existing embeddings: %w", err)
	}
	result.Skipped = skippedCount

	rows, err := db.Query(pendingQuery)
	if err != nil {
		return nil, fmt.Errorf("querying unembedded notes: %w", err)
	}

	type noteText struct {
		id   string
		body string
	}
	var pending []noteText
	for rows.Next() {
		var nt noteText
		if err := rows.Scan(&nt.id, &nt.body); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning unembedded note: %w", err)
		}
		pending = append(pending, nt)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing unembedded rows: %w", err)
	}

	if len(pending) == 0 {
		return result, nil
	}

	// Process in batches of 32.
	const batchSize = 32
	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]

		texts := make([]string, len(batch))
		for j, nt := range batch {
			texts[j] = nt.body
		}
		if isFull {
			// BGE-M3 path: store dense + sparse + ColBERT
			fullOutputs, embedErr := fullEmbedder.EmbedFullBatch(ctx, texts)
			if embedErr != nil {
				log.Debug().Err(embedErr).Int("batch_start", i).Msg("embedding batch failed")
				result.Errors += len(batch)
				continue
			}

			tx, txErr := db.Begin()
			if txErr != nil {
				return nil, fmt.Errorf("beginning embedding transaction: %w", txErr)
			}

			storeErr := false
			for j, out := range fullOutputs {
				noteID := batch[j].id
				if _, err := tx.Exec("UPDATE notes SET embedding = ? WHERE id = ?", EncodeEmbedding(out.Dense), noteID); err != nil {
					log.Debug().Err(err).Str("id", noteID).Msg("storing dense failed")
					storeErr = true
					break
				}
				if _, err := tx.Exec("UPDATE notes SET sparse_embedding = ? WHERE id = ?", EncodeSparseEmbedding(out.Sparse), noteID); err != nil {
					log.Debug().Err(err).Str("id", noteID).Msg("storing sparse failed")
					storeErr = true
					break
				}
				if _, err := tx.Exec("UPDATE notes SET colbert_embedding = ? WHERE id = ?", EncodeColBERTEmbedding(out.ColBERT), noteID); err != nil {
					log.Debug().Err(err).Str("id", noteID).Msg("storing ColBERT failed")
					storeErr = true
					break
				}
			}

			if storeErr {
				_ = tx.Rollback()
				result.Errors += len(batch)
				continue
			}
			if err := tx.Commit(); err != nil {
				_ = tx.Rollback()
				result.Errors += len(batch)
				continue
			}
			result.Embedded += len(batch)
		} else {
			// MiniLM path: dense only
			vectors, embedErr := embedder.EmbedBatch(ctx, texts)
			if embedErr != nil {
				log.Debug().Err(embedErr).Int("batch_start", i).Msg("embedding batch failed")
				result.Errors += len(batch)
				continue
			}

			tx, txErr := db.Begin()
			if txErr != nil {
				return nil, fmt.Errorf("beginning embedding transaction: %w", txErr)
			}

			storeErr := false
			for j, vec := range vectors {
				encoded := EncodeEmbedding(vec)
				if _, err := tx.Exec("UPDATE notes SET embedding = ? WHERE id = ?", encoded, batch[j].id); err != nil {
					log.Debug().Err(err).Str("id", batch[j].id).Msg("failed to store embedding")
					storeErr = true
					break
				}
			}

			if storeErr {
				_ = tx.Rollback()
				result.Errors += len(batch)
				continue
			}
			if err := tx.Commit(); err != nil {
				_ = tx.Rollback()
				log.Debug().Err(err).Int("batch_start", i).Msg("committing embedding batch failed")
				result.Errors += len(batch)
				continue
			}
			result.Embedded += len(batch)
		}
	}

	return result, nil
}

// ResolveLinks updates unresolved links by matching dst_raw against note IDs,
// titles, and aliases. Sets dst_note_id and resolved=TRUE for matches.
func ResolveLinks(db *DB) (int, error) {
	// Resolve by exact ID match
	res1, err := db.Exec(`
		UPDATE OR IGNORE links SET dst_note_id = dst_raw, resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL
		AND dst_raw IN (SELECT id FROM notes)`)
	if err != nil {
		return 0, fmt.Errorf("resolving by ID: %w", err)
	}
	count1, _ := res1.RowsAffected()

	// Resolve by exact title match
	res2, err := db.Exec(`
		UPDATE OR IGNORE links SET dst_note_id = (
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
		UPDATE OR IGNORE links SET dst_note_id = (
			SELECT note_id FROM aliases WHERE alias = links.dst_raw LIMIT 1
		), resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL
		AND dst_raw IN (SELECT alias FROM aliases)`)
	if err != nil {
		return int(count1 + count2), fmt.Errorf("resolving by alias: %w", err)
	}
	count3, _ := res3.RowsAffected()

	// Resolve by filename stem (Obsidian convention).
	// Obsidian wikilinks like [[context-pack|Context Pack]] store dst_raw as
	// the filename stem ("context-pack"). Match against path with directory
	// stripped and .md removed.
	res4, err := db.Exec(`
		UPDATE OR IGNORE links SET dst_note_id = (
			SELECT id FROM notes
			WHERE REPLACE(path, '.md', '') LIKE '%/' || links.dst_raw
			   OR REPLACE(path, '.md', '') = links.dst_raw
			LIMIT 1
		), resolved = TRUE
		WHERE resolved = FALSE AND dst_note_id IS NULL`)
	if err != nil {
		return int(count1 + count2 + count3), fmt.Errorf("resolving by filename: %w", err)
	}
	count4, _ := res4.RowsAffected()

	return int(count1 + count2 + count3 + count4), nil
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
