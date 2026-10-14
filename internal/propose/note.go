package propose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
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
		req.Embed(v.Path)
	}
	return nil
}

func writeFrontmatter(abs string, fm map[string]interface{}, body string) error {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("encoding frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(yamlBytes)
	b.WriteString("---\n")
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	if err := os.WriteFile(abs, []byte(b.String()), 0o640); err != nil { //nolint:gosec // caller resolved the path inside the vault
		return fmt.Errorf("writing note: %w", err)
	}
	return nil
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

func appendSection(body, question, addition string) string {
	question = strings.Join(strings.Fields(question), " ")
	addition = strings.TrimSpace(addition)
	body = strings.TrimRight(body, "\n")
	return body + "\n\n## " + question + "\n\n" + addition + "\n"
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
