package gap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	tmpl "github.com/peiman/vaultmind/internal/template"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
)

var (
	errExists  = errors.New("note already exists")
	errIDTaken = errors.New("id taken")
)

// Open records an open gap, or reports the open gap that already asks this
// question. A closed gap with the same question does not count.
func Open(ctx context.Context, v Vault, req Request, hits Hits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return fail(req, "question_required", "a question is required")
	}
	if v.DB == nil || v.Reg == nil || v.Config == nil {
		return fmt.Errorf("vault is not open")
	}
	existing, ok, err := findOpen(v.DB, req.Question)
	if err != nil {
		log.Debug().Err(err).Msg("gap lookup failed")
		return failOp(req, "lookup_failed", fmt.Errorf("looking for an open gap: %w", err))
	}
	if ok {
		existing.Report, err = Measure(v.DB, v.Path, clock(req.Now))
		if err != nil {
			log.Debug().Err(err).Msg("gap meter failed")
			return failOp(req, "lookup_failed", fmt.Errorf("reading gaps: %w", err))
		}
		return render(req, v, existing)
	}
	if !v.Reg.HasType(vault.GapType) {
		return fail(req, "type_missing", typeMissing)
	}
	ids, err := askedIDs(ctx, hits, req.Question)
	if err != nil {
		log.Debug().Err(err).Msg("gap ask hits failed")
		return failOp(req, "retrieval_failed", fmt.Errorf("listing what ask returns: %w", err))
	}
	written, err := writeNew(ctx, v, req, ids)
	if err != nil {
		return err
	}
	written.Report, err = Measure(v.DB, v.Path, clock(req.Now))
	if err != nil {
		log.Debug().Err(err).Msg("gap meter failed")
		return failOp(req, "lookup_failed", fmt.Errorf("reading gaps: %w", err))
	}
	return render(req, v, written)
}

func askedIDs(ctx context.Context, hits Hits, question string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hits == nil {
		return []string{}, nil
	}
	ids, err := hits.IDs(ctx, question)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		return []string{}, nil
	}
	return ids, nil
}

func findOpen(db *index.DB, question string) (outcome, bool, error) {
	want := normalizeQuestion(question)
	rows, err := db.Query(`
		SELECT n.id, n.path, kv.value_json
		FROM notes n
		JOIN frontmatter_kv kv ON kv.note_id = n.id AND kv.key = 'question'
		WHERE n.type = ? AND n.status = ?
		ORDER BY n.id`, index.TypeGap, statusOpen)
	if err != nil {
		return outcome{}, false, err
	}
	defer func() { _ = rows.Close() }()
	var found outcome
	var matched bool
	for rows.Next() {
		var id, path, raw string
		if err := rows.Scan(&id, &path, &raw); err != nil {
			return outcome{}, false, err
		}
		if matched {
			continue
		}
		var stored string
		if json.Unmarshal([]byte(raw), &stored) != nil {
			continue
		}
		if normalizeQuestion(stored) != want {
			continue
		}
		found = outcome{Mode: modeExisting, ID: id, Path: path, Question: stored, Status: statusOpen}
		matched = true
	}
	if err := rows.Err(); err != nil {
		return outcome{}, false, err
	}
	// The index allows one connection. Close before the next read.
	if err := rows.Close(); err != nil {
		return outcome{}, false, err
	}
	if !matched {
		return outcome{}, false, nil
	}
	hits, err := askedHits(db, found.ID)
	if err != nil {
		return outcome{}, false, err
	}
	found.AskedHits = hits
	return found, true, nil
}

func askedHits(db *index.DB, id string) ([]string, error) {
	var raw string
	err := db.QueryRow(`SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = 'asked_hits'`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		log.Debug().Err(err).Str("id", id).Msg("asked hits lookup failed")
		return nil, fmt.Errorf("reading asked hits: %w", err)
	}
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		log.Debug().Err(err).Str("id", id).Msg("asked hits decode failed")
		return nil, fmt.Errorf("decoding asked hits: %w", err)
	}
	if decoded == nil {
		return []string{}, nil
	}
	return decoded, nil
}

func writeNew(ctx context.Context, v Vault, req Request, ids []string) (outcome, error) {
	now := clock(req.Now)
	slug := slugQuestion(req.Question)
	var last error
	for n := 0; n < 5; n++ {
		rel := gapRel(slug, n)
		out, err := writeGap(ctx, v, req, rel, ids, now)
		if err == nil {
			out.Mode = modeCreated
			return out, nil
		}
		if errors.Is(err, errExists) || errors.Is(err, errIDTaken) {
			last = err
			continue
		}
		return outcome{}, err
	}
	if last == nil {
		last = errExists
	}
	return outcome{}, fmt.Errorf("writing gap: %w", last)
}

func gapRel(slug string, n int) string {
	name := slug
	if n > 0 {
		name = fmt.Sprintf("%s-%d", slug, n+1)
	}
	return gapsDir + "/" + name + ".md"
}

func writeGap(ctx context.Context, v Vault, req Request, rel string, ids []string, now time.Time) (outcome, error) {
	if err := ctx.Err(); err != nil {
		return outcome{}, err
	}
	td, ok := v.Reg.GetTypeDef(vault.GapType)
	if !ok {
		return outcome{}, fmt.Errorf("%s", typeMissing)
	}
	abs, err := vault.ResolveInside(v.Path, rel)
	if err != nil {
		return outcome{}, fmt.Errorf("path %q escapes the vault", rel)
	}
	if _, statErr := os.Stat(abs); statErr == nil {
		return outcome{}, fmt.Errorf("%w: %s", errExists, rel)
	}
	fields := map[string]string{
		"title":     req.Question,
		"status":    statusOpen,
		"question":  req.Question,
		"opened_at": now.Format(time.RFC3339),
	}
	if strings.TrimSpace(req.Session) != "" {
		fields["session"] = strings.TrimSpace(req.Session)
	}
	result, err := tmpl.Process(tmpl.ProcessConfig{
		VaultPath:      v.Path,
		Path:           rel,
		Type:           vault.GapType,
		Fields:         fields,
		AnyFields:      map[string]interface{}{"asked_hits": ids},
		TemplatePath:   "",
		RequiredFields: td.Required,
	})
	if err != nil {
		return outcome{}, fmt.Errorf("processing template: %w", err)
	}
	if existing, idErr := v.DB.QueryNoteByID(result.ID); idErr != nil {
		return outcome{}, fmt.Errorf("checking ID uniqueness: %w", idErr)
	} else if existing != nil {
		return outcome{}, fmt.Errorf("%w: %s at %s", errIDTaken, result.ID, existing.Path)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return outcome{}, fmt.Errorf("creating directories: %w", err)
	}
	if err := os.WriteFile(abs, result.Content, 0o640); err != nil { //nolint:gosec // path is confined with vault.ResolveInside
		return outcome{}, fmt.Errorf("writing note: %w", err)
	}
	if warn := reindex(ctx, v, rel); warn != "" {
		log.Debug().Str("path", rel).Str("warning", warn).Msg("gap reindex failed")
		return outcome{}, fmt.Errorf("%s", warn)
	}
	return outcome{
		Path: rel, ID: result.ID, Question: req.Question, Status: statusOpen, AskedHits: ids,
	}, nil
}

func reindex(ctx context.Context, v Vault, rel string) string {
	if err := ctx.Err(); err != nil {
		return fmt.Sprintf("re-index failed: %s", err)
	}
	dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
	err := index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(rel) //nolint:contextcheck // IndexFile calls Open, which does not accept a context
	if err != nil {
		log.Debug().Err(err).Str("path", rel).Msg("re-index failed")
		return fmt.Sprintf("re-index failed: %s", err)
	}
	return ""
}
