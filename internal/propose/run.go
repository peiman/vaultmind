package propose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/rs/zerolog/log"
)

// Run writes a draft proposal, or reports the draft that already asks this question.
func Run(ctx context.Context, v Vault, req Request, retriever Retriever) error {
	defer closeRanker(req)
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(req.Finding) == "" {
		return fail(req, cmdPropose, "finding_required", "--finding is required")
	}
	if err := bindGap(v, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Question) == "" {
		return fail(req, cmdPropose, "question_required", "question is required")
	}
	if err := validateSources(v.Path, workDir(req), req.Sources); err != nil {
		return fail(req, cmdPropose, "invalid_source", err.Error())
	}
	if v.DB == nil || v.Reg == nil {
		return fmt.Errorf("vault is not open")
	}
	if existing, ok, err := findDraft(v.DB, req.Question); err != nil {
		log.Debug().Err(err).Msg("propose draft lookup failed")
		return failOp(req, cmdPropose, "retrieval_failed", fmt.Errorf("looking for an existing draft: %w", err))
	} else if ok {
		return renderPropose(req, v, existing)
	}
	if retriever == nil {
		return fmt.Errorf("no retriever")
	}
	hit, found, err := retriever.Closest(ctx, v.Path, v.DB, req.Question+"\n"+req.Finding)
	if err != nil {
		log.Debug().Err(err).Msg("propose retrieval failed")
		return failOp(req, cmdPropose, "retrieval_failed", fmt.Errorf("finding the closest note: %w", err))
	}
	now := clock(req)
	if found && bandIsClose(hit.Band, closeBandOrDefault(req.CloseBand)) {
		return writePatch(ctx, v, req, hit, now)
	}
	return writeNew(ctx, v, req, hit, found, now)
}

func findDraft(db *index.DB, question string) (outcome, bool, error) {
	want := normalizeQuestion(question)
	// Patches come back on the same row. A second query while this one is open
	// waits forever: the index allows one connection.
	rows, err := db.Query(`
		SELECT n.id, n.path, kv.value_json, COALESCE(p.value_json, '')
		FROM notes n
		JOIN frontmatter_kv kv ON kv.note_id = n.id AND kv.key = 'question'
		LEFT JOIN frontmatter_kv p ON p.note_id = n.id AND p.key = 'patches'
		WHERE n.status = ?
		ORDER BY n.id`, index.StatusDraft)
	if err != nil {
		return outcome{}, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, path, raw, patchesRaw string
		if err := rows.Scan(&id, &path, &raw, &patchesRaw); err != nil {
			return outcome{}, false, err
		}
		var stored string
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			continue
		}
		if normalizeQuestion(stored) != want {
			continue
		}
		return outcome{Mode: modeExisting, ID: id, Path: path, TargetID: jsonString(patchesRaw)}, true, nil
	}
	return outcome{}, false, rows.Err()
}

func jsonString(raw string) string {
	if raw == "" {
		return ""
	}
	var stored string
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return ""
	}
	return stored
}

func writePatch(ctx context.Context, v Vault, req Request, hit Hit, now time.Time) error {
	if hit.ID == "" || hit.Path == "" {
		return fmt.Errorf("closest note has no id or path")
	}
	noteType := hit.Type
	if noteType == "" || !v.Reg.HasType(noteType) {
		var err error
		noteType, err = chooseType(v.Reg, "")
		if err != nil {
			return err
		}
	}
	rel, err := freeRel(v, proposalRel(hit.Path, hit.ID, req.DraftsDir, now))
	if err != nil {
		return err
	}
	out, err := writeDraft(ctx, v, req, rel, noteType, hit.ID)
	if err != nil {
		return err
	}
	out.Mode = modePatch
	out.TargetID = hit.ID
	if err := attachRankCheck(ctx, v, req, &out, now, hit, true); err != nil {
		return err
	}
	return renderPropose(req, v, out)
}

func writeNew(ctx context.Context, v Vault, req Request, hit Hit, found bool, now time.Time) error {
	hitType := ""
	dir := "."
	if found {
		hitType = hit.Type
		dir = newDir(hit.Path, req.DraftsDir)
	}
	noteType, err := chooseType(v.Reg, hitType)
	if err != nil {
		return err
	}
	slug := slugQuestion(req.Question)
	var last error
	for n := 0; n < 5; n++ {
		rel := newRel(dir, slug, n, now)
		out, err := writeDraft(ctx, v, req, rel, noteType, "")
		if err == nil {
			out.Mode = modeNew
			if err := attachRankCheck(ctx, v, req, &out, now, hit, found); err != nil {
				return err
			}
			return renderPropose(req, v, out)
		}
		if errors.Is(err, errExists) || errors.Is(err, errIDTaken) {
			last = err
			continue
		}
		return err
	}
	return fmt.Errorf("could not place a new note: %w", last)
}
