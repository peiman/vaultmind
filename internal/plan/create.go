package plan

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func CreateNote(vaultPath string, op Operation) (*OpResult, error) {
	absPath := filepath.Join(vaultPath, op.Path)
	cleanVault := filepath.Clean(vaultPath)
	cleanAbs := filepath.Clean(absPath)
	if !strings.HasPrefix(cleanAbs, cleanVault+string(filepath.Separator)) && cleanAbs != cleanVault {
		return nil, &planError{Code: "path_traversal", Message: fmt.Sprintf("path %q escapes vault", op.Path)}
	}
	if _, err := os.Stat(absPath); err == nil {
		return nil, &planError{Code: "path_exists", Message: fmt.Sprintf("file already exists: %s", op.Path)}
	}

	id := ""
	if rawID, ok := op.Frontmatter["id"]; ok {
		if s, ok := rawID.(string); ok && s != "" {
			id = s
		}
	}
	if id == "" {
		id = deriveID(op.Path, op.Type)
	}

	fm := make(map[string]interface{})
	fm["id"] = id
	fm["type"] = op.Type
	for k, v := range op.Frontmatter {
		if k != "id" && k != "type" {
			fm[k] = v
		}
	}
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("serializing frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	if op.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(op.Body)
		if !strings.HasSuffix(op.Body, "\n") {
			buf.WriteString("\n")
		}
	}

	fileBytes := buf.Bytes()
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return nil, fmt.Errorf("creating directories: %w", err)
	}
	if err := os.WriteFile(absPath, fileBytes, 0o600); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	h := sha256.Sum256(fileBytes)
	return &OpResult{Op: OpNoteCreate, Path: op.Path, ID: id, Status: "ok", WriteHash: fmt.Sprintf("sha256:%x", h)}, nil
}

func deriveID(path, noteType string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".md")
	return noteType + "-" + base
}

type planError struct {
	Code    string
	Message string
}

func (e *planError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
