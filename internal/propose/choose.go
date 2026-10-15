package propose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// defaultKnowledgeType is the scaffold's domain-knowledge type: an open status
// list and a required title. A vault that registered it can hold a new draft
// without inventing a type. A new draft does not inherit the import type
// (vault.ImportNoteType); it uses this type when the schema registers it.
const defaultKnowledgeType = "concept"

// defaultDraftsDir is where a draft goes when it must not sit in an import
// folder. The registry key app.notepropose.drafts_dir overrides it. "." means
// the vault root. A drafts dir that is itself under the import folder falls
// back here.
const defaultDraftsDir = "notes"

// bandIsClose reports whether the band ask already computed is at least the
// band this vault treats as close enough to patch. strong is closer than
// moderate, then weak, then no_match. A weaker band, an empty band, or a
// typo'd setting is not close, so the finding becomes a new note instead of
// landing on the wrong one.
func bandIsClose(band, closeBand string) bool {
	got, ok := bandRank(band)
	want, wantOK := bandRank(closeBand)
	if !ok || !wantOK {
		return false
	}
	return got >= want
}

func bandRank(band string) (int, bool) {
	switch band {
	case noisefloor.ConfidenceStrong:
		return 3, true
	case noisefloor.ConfidenceModerate:
		return 2, true
	case noisefloor.ConfidenceWeak:
		return 1, true
	case noisefloor.ConfidenceNoMatch:
		return 0, true
	default:
		return 0, false
	}
}

// closeBandOrDefault uses the strong band when the setting is empty. Only a
// strong match is close enough to attach a finding to an existing note.
func closeBandOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return noisefloor.ConfidenceStrong
	}
	return strings.TrimSpace(s)
}

func normalizeQuestion(q string) string {
	return strings.ToLower(strings.Join(strings.Fields(q), " "))
}

func slugQuestion(q string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(q)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if !hyphen && b.Len() > 0 {
			b.WriteByte('-')
			hyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "note"
	}
	const maxSlug = 80
	if len(s) > maxSlug {
		s = strings.Trim(s[:maxSlug], "-")
	}
	if s == "" {
		s = "note"
	}
	return s
}

// proposalPath names a patch draft. The id comes from this name, and the
// accepted marker carries the id, so the name must never repeat for a
// different finding: it has the second it was made and a short hash of the
// finding. A minute-resolution name let a later proposal reuse an accepted
// one's id, and accept then skipped it as already applied.
func proposalPath(targetPath, targetID, finding string, now time.Time) string {
	name := targetID + ".proposal-" + now.UTC().Format("20060102-150405") + "-" + findingHash(finding) + ".md"
	dir := filepath.ToSlash(filepath.Dir(targetPath))
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// proposalRel keeps a patch beside its target. A target under the import
// folder cannot hold the proposal: the draft goes in the drafts folder, with
// the same filename.
func proposalRel(targetPath, targetID, drafts, finding string, now time.Time) string {
	beside := proposalPath(targetPath, targetID, finding, now)
	if !underImport(targetPath) {
		return beside
	}
	name := beside
	if i := strings.LastIndex(beside, "/"); i >= 0 {
		name = beside[i+1:]
	}
	return joinRel(draftsDir(drafts), name)
}

// findingHash is the first six hex digits of the finding's SHA-256, after
// collapsing whitespace, so the same finding re-typed names the same draft.
func findingHash(finding string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(finding), " ")))
	return hex.EncodeToString(sum[:])[:6]
}

func joinRel(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// underImport reports a path that is the import folder or inside it.
// imported.md at the vault root is not. The comparison is case-sensitive,
// matching the folder the import command writes.
func underImport(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	folder := vault.ImportFolder
	return rel == folder || strings.HasPrefix(rel, folder+"/")
}

// draftsDir cleans the setting before judging it, so "notes/../imported" or
// "./imported" is seen as the import folder and "../x" or "/x" as outside the
// vault. Any of those falls back to the default drafts folder.
func draftsDir(s string) string {
	s = filepath.ToSlash(strings.TrimSpace(s))
	if s == "" || strings.HasPrefix(s, "/") {
		return defaultDraftsDir
	}
	s = path.Clean(s)
	if s == "." {
		return "."
	}
	if s == ".." || strings.HasPrefix(s, "../") || underImport(s) {
		return defaultDraftsDir
	}
	return s
}

func newDir(hitPath, drafts string) string {
	if underImport(hitPath) {
		return draftsDir(drafts)
	}
	return hitDir(hitPath)
}

func hitDir(rel string) string {
	if rel == "" {
		return "."
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." || dir == "" {
		return "."
	}
	return dir
}

func newRel(dir, slug string, n int, now time.Time) string {
	name := slug
	if n > 0 {
		name = slug + "-" + now.UTC().Format("20060102-1504")
		if n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
	}
	if dir == "." || dir == "" {
		return name + ".md"
	}
	return dir + "/" + name + ".md"
}

// chooseType picks the type of a new draft.
//
// A top hit with a registered type lends that type, including when the band is
// not close enough to patch, except the type the import command writes.
// Otherwise the vault's default knowledge type is
// the registered type named concept. If the vault has no concept, the first
// type in alphabetical order that can hold a draft is used, skipping the
// built-in overview type. A closed list that rejects draft is skipped here so
// the new note is one the schema can accept.
func chooseType(reg *schema.Registry, hitType string) (string, error) {
	// The import type is a projection of someone else's doc. A new answer is
	// knowledge, so it does not inherit that type even when the schema
	// registers it (Principle 7).
	if hitType != "" && hitType != vault.ImportNoteType && reg.HasType(hitType) {
		return hitType, nil
	}
	if reg.HasType(defaultKnowledgeType) {
		return defaultKnowledgeType, nil
	}
	for _, name := range reg.ListTypes() {
		if name == vault.OverviewType || name == vault.GapType || name == vault.ImportNoteType {
			continue
		}
		if reg.ValidStatus(name, index.StatusDraft) {
			return name, nil
		}
	}
	return "", fmt.Errorf("vault schema has no type for a new note")
}

func nextStep(id string) string {
	return fmt.Sprintf("a person reviews it; accept with `vaultmind note accept %s`", id)
}

func clock(req Request) time.Time {
	if req.Now.IsZero() {
		return time.Now().UTC()
	}
	return req.Now.UTC()
}
