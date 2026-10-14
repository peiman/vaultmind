package propose

import (
	"fmt"
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
// without inventing a type.
const defaultKnowledgeType = "concept"

// bandIsClose reports whether the band ask already computed is the band this
// vault treats as close enough to patch. Equality is the whole rule: a weaker
// band, an empty band, or a typo'd setting is not close, so the finding becomes
// a new note instead of landing on the wrong one.
func bandIsClose(band, closeBand string) bool {
	return band == closeBand
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

func proposalPath(targetPath, targetID string, now time.Time) string {
	name := targetID + ".proposal-" + now.UTC().Format("20060102-1504") + ".md"
	dir := filepath.ToSlash(filepath.Dir(targetPath))
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func hitDir(path string) string {
	if path == "" {
		return "."
	}
	dir := filepath.ToSlash(filepath.Dir(path))
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
// not close enough to patch. Otherwise the vault's default knowledge type is
// the registered type named concept. If the vault has no concept, the first
// type in alphabetical order that can hold a draft is used, skipping the
// built-in overview type. A closed list that rejects draft is skipped here so
// the new note is one the schema can accept.
func chooseType(reg *schema.Registry, hitType string) (string, error) {
	if hitType != "" && reg.HasType(hitType) {
		return hitType, nil
	}
	if reg.HasType(defaultKnowledgeType) {
		return defaultKnowledgeType, nil
	}
	for _, name := range reg.ListTypes() {
		if name == vault.OverviewType {
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
