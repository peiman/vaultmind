package propose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	tmpl "github.com/peiman/vaultmind/internal/template"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var (
	errExists  = errors.New("note already exists")
	errIDTaken = errors.New("id taken")
)

func findingBody(finding string, sources []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(finding))
	b.WriteString("\n\n## Sources\n")
	for _, source := range sources {
		b.WriteString("- ")
		b.WriteString(source)
		b.WriteByte('\n')
	}
	return b.String()
}

func writeDraft(ctx context.Context, v Vault, req Request, rel, noteType, patches string) (outcome, error) {
	if !v.Reg.HasType(noteType) {
		return outcome{}, fmt.Errorf("unknown type %q (registered: %s)", noteType, strings.Join(v.Reg.ListTypes(), ", "))
	}
	td, _ := v.Reg.GetTypeDef(noteType)
	templatePath := ""
	if td.Template != "" {
		var err error
		if templatePath, err = vault.ResolveInside(v.Path, td.Template); err != nil {
			return outcome{}, fmt.Errorf("type %q template: %w", noteType, err)
		}
	}
	abs, err := vault.ResolveInside(v.Path, rel)
	if err != nil {
		return outcome{}, fmt.Errorf("path %q escapes the vault", rel)
	}
	if _, statErr := os.Stat(abs); statErr == nil {
		return outcome{}, fmt.Errorf("%w: %s", errExists, rel)
	}

	fields := map[string]string{
		"title":    req.Question,
		"status":   index.StatusDraft,
		"question": req.Question,
	}
	if patches != "" {
		fields["patches"] = patches
	}
	if strings.TrimSpace(req.GapID) != "" {
		fields["closes_gap"] = strings.TrimSpace(req.GapID)
	}
	sources := req.Sources
	if sources == nil {
		sources = []string{}
	}
	result, err := tmpl.Process(tmpl.ProcessConfig{
		VaultPath:      v.Path,
		Path:           rel,
		Type:           noteType,
		Fields:         fields,
		Body:           findingBody(req.Finding, req.Sources),
		TemplatePath:   templatePath,
		RequiredFields: td.Required,
		AnyFields:      map[string]interface{}{"sources": sources},
	})
	if err != nil {
		return outcome{}, fmt.Errorf("processing template: %w", err)
	}
	for _, required := range td.Required {
		val, exists := result.FinalFrontmatter[required]
		if !exists || val == nil || val == "" {
			return outcome{}, fmt.Errorf("required field %q is missing or empty", required)
		}
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
	warnings := append([]string{}, result.Warnings...)
	warnings = append(warnings, reindex(ctx, v, req, rel)...)
	return outcome{Path: rel, ID: result.ID, Warnings: warnings}, nil
}

func reindex(ctx context.Context, v Vault, req Request, rel string) []string {
	if err := ctx.Err(); err != nil {
		return []string{fmt.Sprintf("re-index failed: %s", err)}
	}
	if v.Config == nil {
		return []string{"re-index failed: vault config is missing"}
	}
	dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
	// IndexFile opens the index through Open. Open does not accept a context;
	// applyMigrations runs goose with context.Background. Same boundary as the
	// Open calls in internal/index/indexer.go.
	err := index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(rel) //nolint:contextcheck // IndexFile calls Open, which does not accept a context
	if err != nil {
		log.Debug().Err(err).Str("path", rel).Msg("re-index failed")
		return []string{fmt.Sprintf("re-index failed: %s", err)}
	}
	if req.Embed != nil {
		req.Embed(v.Path, rel)
	}
	return nil
}

// acceptedMarker is the invisible line accept writes in front of a proposal's
// section. A retry that finds it skips the append and only finishes cleanup.
func acceptedMarker(proposalID string) string {
	return "<!-- vaultmind:accepted " + proposalID + " -->"
}

// rewriteNote changes frontmatter through the mutation package and writes the
// file with its conflict-checked atomic rename. edit may be nil. newBody, when
// non-nil, replaces the body; nil keeps the body bytes.
func rewriteNote(abs, rel string, raw []byte, hook func(string) error, edit func(*yaml.Node) error, newBody *string) error {
	doc, bodyOffset, err := mutation.ParseFrontmatterNode(raw)
	if err != nil {
		return fmt.Errorf("parsing note %s: %w", rel, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("frontmatter of %s is not a YAML mapping", rel)
	}
	var fm []byte
	if edit == nil {
		fm = append([]byte(nil), raw[:bodyOffset]...)
	} else {
		if err := edit(doc.Content[0]); err != nil {
			return err
		}
		fm, err = mutation.SerializeFrontmatter(doc, mutation.DetectLineEnding(raw))
		if err != nil {
			return fmt.Errorf("encoding frontmatter: %w", err)
		}
	}
	file := raw
	if newBody != nil {
		file = append(append([]byte{}, raw[:bodyOffset]...), []byte(*newBody)...)
	}
	content := mutation.SpliceFile(file, fm, bodyOffset)
	if hook != nil {
		if err := hook(abs); err != nil {
			return err
		}
	}
	if err := mutation.AtomicWrite(abs, rel, content, mutation.FileHash(raw)); err != nil {
		return noteWriteErr(rel, err)
	}
	return nil
}

func noteWriteErr(rel string, err error) error {
	var me *mutation.MutationError
	if errors.As(err, &me) && me.Code == "conflict" {
		return fmt.Errorf("note %s changed while it was being accepted; nothing was written", rel)
	}
	return fmt.Errorf("writing note %s: %w", rel, err)
}

func fmString(fm map[string]interface{}, key string) string {
	v, ok := fm[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprint(v)
}

func stringList(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string{}, t...)
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func mergeSources(existing, add []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, source := range append(existing, add...) {
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	return out
}

func appendSection(body, question, addition, proposalID string) string {
	question = strings.Join(strings.Fields(question), " ")
	addition = strings.TrimSpace(addition)
	body = strings.TrimRight(body, "\n")
	return body + "\n\n" + acceptedMarker(proposalID) + "\n## " + question + "\n\n" + addition + "\n"
}

func freeRel(v Vault, rel string) (string, error) {
	base := strings.TrimSuffix(rel, ".md")
	for i := 0; i < 20; i++ {
		candidate := rel
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d.md", base, i+1)
		}
		abs, err := vault.ResolveInside(v.Path, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free path near %s", rel)
}
