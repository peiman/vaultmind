package propose

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
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
	row, err := lookupAcceptNote(ctx, v, req.ID)
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
		return missingAcceptFile(req, v, row, err)
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
	return acceptNew(ctx, v, req, row.Path, row.ID, parsed, abs, raw)
}

func acceptNew(ctx context.Context, v Vault, req Request, rel, id string, parsed *parser.ParsedNote, abs string, raw []byte) error {
	noteType := fmString(parsed.Frontmatter, "type")
	td, ok := v.Reg.GetTypeDef(noteType)
	if !ok {
		return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q has type %q, which is not in the vault schema", id, noteType))
	}
	published, err := publishStatus(td)
	if err != nil {
		return fail(req, cmdAccept, "note_missing", err.Error())
	}
	err = rewriteNote(abs, rel, raw, req.beforeWrite, func(mapping *yaml.Node) error {
		return mutation.SetKey(mapping, "status", published)
	}, nil)
	if err != nil {
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
	if err := refusePatchTarget(req, parsed, targetID); err != nil {
		return err
	}
	// The marker is the record that this proposal was already copied onto the
	// target. A retry after a failed delete must not append the section again.
	if !bytes.Contains(raw, []byte(acceptedMarker(req.ID))) {
		if err := applyPatch(abs, target.Path, raw, parsed, proposal, req, v.Reg.IsFieldAllowed(target.Type, "sources")); err != nil {
			return err
		}
	}
	proposalAbs, err := vault.ResolveInside(v.Path, proposalRel)
	if err != nil {
		return err
	}
	// The row goes first. If this fails, the file is still there and a retry
	// reads it. If the file delete fails after this, a retry finds the file
	// (re-indexed, or by path) and only finishes cleanup.
	if err := removeProposalRow(req, v.DB, proposalRel); err != nil {
		return fmt.Errorf("removing proposal %s from the index: %w", proposalRel, err)
	}
	if err := removeProposalFile(req, proposalAbs); err != nil {
		return fmt.Errorf("removing proposal %s: %w", proposalRel, err)
	}
	return renderAccept(req, v, outcome{
		Mode: modePatch, Path: target.Path, ID: target.ID, TargetID: target.ID,
		Removed: proposalRel, Warnings: reindex(ctx, v, req, target.Path),
	})
}

func refusePatchTarget(req Request, note *parser.ParsedNote, targetID string) error {
	if fmString(note.Frontmatter, "patches") != "" {
		return fail(req, cmdAccept, "target_draft", fmt.Sprintf("proposal %q patches %q, which is itself a proposal", req.ID, targetID))
	}
	if fmString(note.Frontmatter, "status") == index.StatusDraft {
		return fail(req, cmdAccept, "target_draft", fmt.Sprintf("proposal %q patches %q, which is a draft; accept that note first", req.ID, targetID))
	}
	return nil
}

func applyPatch(abs, rel string, raw []byte, target, proposal *parser.ParsedNote, req Request, sourcesAllowed bool) error {
	_, bodyOffset, err := mutation.ParseFrontmatterNode(raw)
	if err != nil {
		return fmt.Errorf("parsing note %s: %w", rel, err)
	}
	body := appendSection(string(raw[bodyOffset:]), fmString(proposal.Frontmatter, "question"), proposal.Body, req.ID)
	var edit func(*yaml.Node) error
	add := stringList(proposal.Frontmatter["sources"])
	if sourcesAllowed && len(add) > 0 {
		merged := mergeSources(stringList(target.Frontmatter["sources"]), add)
		edit = func(mapping *yaml.Node) error {
			return mutation.SetKey(mapping, "sources", merged)
		}
	}
	return rewriteNote(abs, rel, raw, req.beforeWrite, edit, &body)
}

// lookupAcceptNote returns the indexed draft. The row is deleted before the
// file, so a retry after a failed file delete finds no row: the file is
// re-indexed and cleanup finishes. A missing id stays a missing id.
func lookupAcceptNote(ctx context.Context, v Vault, id string) (*index.NoteRow, error) {
	row, err := v.DB.QueryNoteByID(id)
	if err != nil || row != nil {
		return row, err
	}
	return reindexUnindexedProposal(ctx, v, id)
}

func missingAcceptFile(req Request, v Vault, row *index.NoteRow, err error) error {
	if os.IsNotExist(err) {
		return finishMissingProposal(req, v, row)
	}
	return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q is not in the vault", req.ID))
}

func deleteProposal(path string) error {
	return os.Remove(path) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
}

func removeProposalRow(req Request, db *index.DB, rel string) error {
	var err error
	if req.deleteIndexed != nil {
		err = req.deleteIndexed(db, rel)
	} else {
		err = index.DeleteNoteByPath(db, rel)
	}
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Err(err).Str("path", rel).Msg("proposal row already gone")
		return nil
	}
	return err
}

func removeProposalFile(req Request, path string) error {
	if req.removeFile != nil {
		return req.removeFile(path)
	}
	return deleteProposal(path)
}

// finishMissingProposal removes the index row of a proposal whose file is
// already gone, and says so. Any other missing file is still "not in the vault":
// a draft that is not a proposal is left indexed.
func finishMissingProposal(req Request, v Vault, row *index.NoteRow) error {
	proposal, err := indexedProposal(v.DB, row.ID)
	if err != nil {
		log.Debug().Err(err).Str("id", row.ID).Msg("accept proposal lookup failed")
		return failOp(req, cmdAccept, "note_missing", fmt.Errorf("reading note %q: %w", req.ID, err))
	}
	if !proposal {
		return fail(req, cmdAccept, "note_missing", fmt.Sprintf("note %q is not in the vault", req.ID))
	}
	if err := removeProposalRow(req, v.DB, row.Path); err != nil {
		return fmt.Errorf("removing proposal %s from the index: %w", row.Path, err)
	}
	return renderProposalGone(req, v, row)
}

func indexedProposal(db *index.DB, id string) (bool, error) {
	var raw string
	err := db.QueryRow(`SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = 'patches'`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return patchesNonEmpty(raw), nil
}

// patchesNonEmpty reports a stored patches value. A value that is not a JSON
// string is not a proposal: the row stays, and accept does not delete it.
func patchesNonEmpty(raw string) bool {
	var value string
	if json.Unmarshal([]byte(raw), &value) != nil {
		return false
	}
	return strings.TrimSpace(value) != ""
}

// reindexUnindexedProposal finds a proposal file the index no longer has.
// Accept deletes the row before the file, so a failed file delete leaves the
// file on disk and the id unknown. The retry reads that file, sees the marker
// on the target, and only finishes cleanup.
func reindexUnindexedProposal(ctx context.Context, v Vault, id string) (*index.NoteRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if v.Config == nil {
		return nil, nil //nolint:nilnil // no vault config: treat the id as not found
	}
	scanned, err := vault.Scan(v.Path, v.Config.Vault.Exclude)
	if err != nil {
		return nil, fmt.Errorf("looking for proposal %s: %w", id, err)
	}
	for _, file := range scanned.Files {
		abs, err := vault.ResolveInside(v.Path, file.RelPath)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(abs) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
		if err != nil {
			continue
		}
		parsed, err := parser.Parse(raw)
		if err != nil || fmString(parsed.Frontmatter, "id") != id {
			continue
		}
		if fmString(parsed.Frontmatter, "status") != index.StatusDraft || fmString(parsed.Frontmatter, "patches") == "" {
			return nil, nil //nolint:nilnil // the file is this id and is not a proposal
		}
		dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
		// IndexFile opens the index through Open. Open does not accept a context.
		// Same boundary as reindex.
		if err := index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(file.RelPath); err != nil { //nolint:contextcheck // IndexFile calls Open, which does not accept a context
			return nil, fmt.Errorf("re-indexing proposal %s: %w", file.RelPath, err)
		}
		log.Debug().Str("id", id).Str("path", file.RelPath).Msg("re-indexed proposal whose row was missing")
		return v.DB.QueryNoteByID(id)
	}
	return nil, nil //nolint:nilnil // no proposal file has this id
}

func renderProposalGone(req Request, v Vault, row *index.NoteRow) error {
	if !req.JSON {
		_, err := fmt.Fprintf(stdout(req), "proposal %s was already gone; removed its index row\n", req.ID)
		return err
	}
	return writeEnvelope(req, v, envelope.OK(cmdAccept, acceptResult{
		Mode: "cleanup", Path: row.Path, ID: req.ID,
	}), nil)
}
