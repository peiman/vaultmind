package propose

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
)

type proposeResult struct {
	Mode      string     `json:"mode"`
	Path      string     `json:"path"`
	ID        string     `json:"id"`
	TargetID  string     `json:"target_id,omitempty"`
	NextStep  string     `json:"next_step"`
	RankCheck *RankCheck `json:"rank_check,omitempty"`
}

type acceptResult struct {
	Mode     string `json:"mode"`
	Path     string `json:"path"`
	ID       string `json:"id"`
	TargetID string `json:"target_id,omitempty"`
	Status   string `json:"status,omitempty"`
}

func fail(req Request, command, code, msg string) error {
	if !req.JSON {
		return fmt.Errorf("%s", msg)
	}
	w := req.Stdout
	if w == nil {
		w = io.Discard
	}
	return envelope.WriteError(w, envelope.Error(command, code, msg, ""))
}

func failOp(req Request, command, code string, err error) error {
	if req.JSON {
		return fail(req, command, code, err.Error())
	}
	return err
}

func stdout(req Request) io.Writer {
	if req.Stdout == nil {
		return io.Discard
	}
	return req.Stdout
}

func renderPropose(req Request, v Vault, o outcome) error {
	o.Mode = strings.TrimSpace(o.Mode)
	next := nextStep(o.ID)
	if !req.JSON {
		writeWarnings(req, o.Warnings)
		var b strings.Builder
		fmt.Fprintf(&b, "mode: %s\npath: %s\nid: %s\n", o.Mode, o.Path, o.ID)
		if o.TargetID != "" {
			fmt.Fprintf(&b, "target: %s\n", o.TargetID)
		}
		if o.Rank != nil {
			b.WriteString(formatRankCheck(*o.Rank))
			b.WriteByte('\n')
		}
		b.WriteString(next)
		b.WriteByte('\n')
		_, err := io.WriteString(stdout(req), b.String())
		return err
	}
	env := envelope.OK(cmdPropose, proposeResult{
		Mode: o.Mode, Path: o.Path, ID: o.ID, TargetID: o.TargetID, NextStep: next, RankCheck: o.Rank,
	})
	return writeEnvelope(req, v, env, o.Warnings)
}

func renderAccept(req Request, v Vault, o outcome) error {
	// A missing linked gap is its own warning. writeEnvelope tags every
	// string in Warnings as reindex_failed, and this one is not a reindex.
	textWarnings := o.Warnings
	if o.MissingGap != "" {
		textWarnings = append([]string{missingGapWarning(o.MissingGap)}, o.Warnings...)
	}
	if !req.JSON {
		writeWarnings(req, textWarnings)
		var err error
		if o.Mode == modePatch {
			_, err = fmt.Fprintf(stdout(req), "accepted %s — added the proposal to it and removed %s\n", o.ID, o.Removed)
		} else {
			_, err = fmt.Fprintf(stdout(req), "accepted %s (status: %s)\n", o.ID, o.Status)
		}
		return err
	}
	result := acceptResult{Mode: o.Mode, Path: o.Path, ID: o.ID, Status: o.Status}
	if o.Mode == modePatch {
		result.TargetID = o.TargetID
		result.Status = ""
	}
	env := envelope.OK(cmdAccept, result)
	if o.MissingGap != "" {
		env.AddWarning("gap_missing", missingGapWarning(o.MissingGap), o.MissingGap)
	}
	return writeEnvelope(req, v, env, o.Warnings)
}

func writeWarnings(req Request, warnings []string) {
	if req.Stderr == nil {
		return
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(req.Stderr, "warning: %s\n", warning)
	}
}

func writeEnvelope(req Request, v Vault, env *envelope.Envelope, warnings []string) error {
	for _, warning := range warnings {
		env.AddWarning("reindex_failed", warning, "")
	}
	env.Meta.VaultPath = v.Path
	if v.IndexHash != nil {
		env.Meta.IndexHash = v.IndexHash()
	}
	return json.NewEncoder(stdout(req)).Encode(env)
}
