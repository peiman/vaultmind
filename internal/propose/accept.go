package propose

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
)

// publishWords is the order in which a type's allowed statuses are asked
// which word means "no longer a draft". accepted is the word ADR-102 uses.
// active is the word a project list uses. Anything else is not guessed.
var publishWords = []string{"accepted", "active"}

func publishStatus(td vault.TypeDef) (string, error) {
	if len(td.Statuses) == 0 {
		return "accepted", nil
	}
	allowed := make(map[string]bool, len(td.Statuses))
	for _, status := range td.Statuses {
		allowed[status] = true
	}
	for _, word := range publishWords {
		if allowed[word] {
			return word, nil
		}
	}
	return "", fmt.Errorf("type has no accepted or active status to publish a draft")
}

// Accept promotes a draft. A new note flips status in the same file. A patch
// is appended to its target and the proposal file is removed, so one copy remains.
func Accept(ctx context.Context, v Vault, req Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(req.ID) == "" {
		return fail(req, cmdAccept, "note_missing", "usage: vaultmind note accept <id>")
	}
	if v.DB == nil || v.Reg == nil {
		return fmt.Errorf("vault is not open")
	}
	row, err := v.DB.QueryNoteByID(req.ID)
	if err != nil {
		log.Debug().Err(err).Str("id", req.ID).Msg("accept lookup failed")
		return failOp(req, cmdAccept, "note_missing", fmt.Errorf("reading note %q: %w", req.ID, err))
	}
	if row == nil {
		return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q is not in the vault", req.ID))
	}
	abs, err := vault.ResolveInside(v.Path, row.Path)
	if err != nil {
		return fail(req, cmdAccept, "note_missing", err.Error())
	}
	raw, err := os.ReadFile(abs) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
	if err != nil {
		return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q is not in the vault", req.ID))
	}
	parsed, err := parser.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing note %q: %w", req.ID, err)
	}
	status := fmString(parsed.Frontmatter, "status")
	if status != index.StatusDraft {
		return fail(req, cmdAccept, "not_draft", fmt.Sprintf("note %q is not a draft (status %q); accept only promotes a draft", req.ID, status))
	}
	if patches := fmString(parsed.Frontmatter, "patches"); patches != "" {
		return acceptPatch(ctx, v, req, row.Path, parsed, patches)
	}
	return acceptNew(ctx, v, req, row.Path, row.ID, parsed, abs)
}

func acceptNew(ctx context.Context, v Vault, req Request, rel, id string, parsed *parser.ParsedNote, abs string) error {
	noteType := fmString(parsed.Frontmatter, "type")
	td, ok := v.Reg.GetTypeDef(noteType)
	if !ok {
		return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q has type %q, which is not in the vault schema", id, noteType))
	}
	published, err := publishStatus(td)
	if err != nil {
		return fail(req, cmdAccept, "note_missing", err.Error())
	}
	parsed.Frontmatter["status"] = published
	if err := writeFrontmatter(abs, parsed.Frontmatter, parsed.Body); err != nil {
		return err
	}
	return renderAccept(req, v, outcome{
		Mode: modeNew, Path: rel, ID: id, Status: published, Warnings: reindex(ctx, v, req, rel),
	})
}

func acceptPatch(ctx context.Context, v Vault, req Request, proposalRel string, proposal *parser.ParsedNote, targetID string) error {
	target, err := v.DB.QueryNoteByID(targetID)
	if err != nil {
		log.Debug().Err(err).Str("id", targetID).Msg("accept target lookup failed")
		return failOp(req, cmdAccept, "target_missing", fmt.Errorf("reading note %q: %w", targetID, err))
	}
	if target == nil {
		return fail(req, cmdAccept, "target_missing", fmt.Sprintf("proposal %q patches %q, which is not in the vault", req.ID, targetID))
	}
	abs, err := vault.ResolveInside(v.Path, target.Path)
	if err != nil {
		return fail(req, cmdAccept, "target_missing", err.Error())
	}
	raw, err := os.ReadFile(abs) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
	if err != nil {
		return fail(req, cmdAccept, "target_missing", fmt.Sprintf("proposal %q patches %q, which is not in the vault", req.ID, targetID))
	}
	parsed, err := parser.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing note %q: %w", targetID, err)
	}
	question := fmString(proposal.Frontmatter, "question")
	parsed.Body = appendSection(parsed.Body, question, proposal.Body)
	if v.Reg.IsFieldAllowed(target.Type, "sources") {
		add := stringList(proposal.Frontmatter["sources"])
		if len(add) > 0 {
			parsed.Frontmatter["sources"] = mergeSources(stringList(parsed.Frontmatter["sources"]), add)
		}
	}
	if err := writeFrontmatter(abs, parsed.Frontmatter, parsed.Body); err != nil {
		return err
	}
	proposalAbs, err := vault.ResolveInside(v.Path, proposalRel)
	if err != nil {
		return err
	}
	if err := deleteProposal(proposalAbs); err != nil {
		return fmt.Errorf("removing proposal %s: %w", proposalRel, err)
	}
	if err := index.DeleteNoteByPath(v.DB, proposalRel); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("removing proposal %s from the index: %w", proposalRel, err)
	} else if err != nil {
		log.Debug().Err(err).Str("path", proposalRel).Msg("proposal row already gone")
	}
	return renderAccept(req, v, outcome{
		Mode: modePatch, Path: target.Path, ID: target.ID, TargetID: target.ID,
		Removed: proposalRel, Warnings: reindex(ctx, v, req, target.Path),
	})
}

func deleteProposal(path string) error {
	return os.Remove(path)
}
