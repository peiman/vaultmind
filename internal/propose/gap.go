package propose

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	gapStatusOpen   = "open"
	gapStatusClosed = "closed"
)

// bindGap checks an open gap and, when the caller gave no question, uses
// the one stored on it. A missing, closed, or non-gap id fails before a
// draft is written.
func bindGap(v Vault, req *Request) error {
	if strings.TrimSpace(req.GapID) == "" {
		return nil
	}
	req.GapID = strings.TrimSpace(req.GapID)
	if v.DB == nil {
		return fmt.Errorf("vault is not open")
	}
	question, err := openGapQuestion(v, req.GapID)
	if err != nil {
		return fail(*req, cmdPropose, "gap_missing", err.Error())
	}
	if strings.TrimSpace(req.Question) == "" {
		req.Question = question
	}
	return nil
}

func openGapQuestion(v Vault, id string) (string, error) {
	note, err := v.DB.QueryFullNote(id)
	if err != nil {
		log.Debug().Err(err).Str("id", id).Msg("gap lookup failed")
		return "", fmt.Errorf("reading gap %q: %w", id, err)
	}
	if note == nil {
		return "", fmt.Errorf("gap %q is not in the vault", id)
	}
	if note.Type != index.TypeGap {
		return "", fmt.Errorf("note %q is not a gap", id)
	}
	if fmString(note.Frontmatter, "status") != gapStatusOpen {
		return "", fmt.Errorf("gap %q is not open", id)
	}
	question := strings.TrimSpace(fmString(note.Frontmatter, "question"))
	if question == "" {
		return "", fmt.Errorf("gap %q has no question", id)
	}
	return question, nil
}

// missingGapWarning is the one sentence for a closes_gap id that is not in
// the vault. Text and JSON both use it. The note is still accepted: the
// finding is the knowledge, and a missing gap must not leave it a draft.
func missingGapWarning(id string) string {
	return fmt.Sprintf("gap %q is not in the vault; accepted the note anyway", id)
}

// closeLinkedGap flips the gap named by closes_gap. closedBy is the accepted
// note: the draft itself, or the note a patch lands on. Already closed is a
// no-op, so a retry after a failed accept finishes the draft and leaves the
// gap closed. The gap is closed before the draft changes, so a failed accept
// can be retried while the draft is still a draft.
//
// A confirmed absence (the id is not in the index) returns that id and a nil
// error. Accept publishes the note and warns. A lookup error is not absence:
// the gap might still be there, so accept fails with gap_missing. A note that
// is not a gap, a path outside the vault, and a file that cannot be parsed
// fail with gap_invalid. A file that cannot be read fails with gap_unreadable.
// Those failures leave the draft a draft.
func closeLinkedGap(ctx context.Context, v Vault, req Request, note *parser.ParsedNote, closedBy string) (string, error) {
	gapID := fmString(note.Frontmatter, "closes_gap")
	if strings.TrimSpace(gapID) == "" {
		return "", nil
	}
	gapID = strings.TrimSpace(gapID)
	row, err := v.DB.QueryNoteByID(gapID)
	if err != nil {
		log.Debug().Err(err).Str("id", gapID).Msg("close gap lookup failed")
		return "", failOp(req, cmdAccept, "gap_missing", fmt.Errorf("reading gap %q: %w", gapID, err))
	}
	if row == nil {
		return gapID, nil
	}
	if row.Type != index.TypeGap {
		return "", failOp(req, cmdAccept, "gap_invalid", fmt.Errorf("note %q is not a gap", gapID))
	}
	abs, err := vault.ResolveInside(v.Path, row.Path)
	if err != nil {
		return "", failOp(req, cmdAccept, "gap_invalid", err)
	}
	raw, err := os.ReadFile(abs) //nolint:gosec // nosemgrep: go-path-traversal -- path is confined with vault.ResolveInside
	if err != nil {
		return "", failOp(req, cmdAccept, "gap_unreadable", fmt.Errorf("reading gap %q: %w", gapID, err))
	}
	parsed, err := parser.Parse(raw)
	if err != nil {
		return "", failOp(req, cmdAccept, "gap_invalid", fmt.Errorf("parsing gap %q: %w", gapID, err))
	}
	if fmString(parsed.Frontmatter, "status") == gapStatusClosed {
		if row.Status != gapStatusClosed {
			_ = reindex(ctx, v, req, row.Path)
		}
		return "", nil
	}
	closedAt := clock(req).UTC().Format(time.RFC3339)
	err = rewriteNote(abs, row.Path, raw, nil, func(mapping *yaml.Node) error {
		if err := mutation.SetKey(mapping, "status", gapStatusClosed); err != nil {
			return err
		}
		if err := mutation.SetKey(mapping, "closed_at", closedAt); err != nil {
			return err
		}
		return mutation.SetKey(mapping, "closed_by", closedBy)
	}, nil)
	if err != nil {
		return "", err
	}
	_ = reindex(ctx, v, req, row.Path)
	return "", nil
}
