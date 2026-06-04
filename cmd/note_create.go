package cmd

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/index"
	tmpl "github.com/peiman/vaultmind/internal/template"
	"github.com/spf13/cobra"
)

var noteCreateCmd = MustNewCommand(commands.NoteCreateMetadata, runNoteCreate)

func init() {
	noteCmd.AddCommand(noteCreateCmd)
	setupCommandConfig(noteCreateCmd)
	noteCreateCmd.Flags().StringSlice("field", nil, "Set frontmatter field (key=value, repeatable)")
	noteCreateCmd.Flags().Bool("body-stdin", false, "Read body content from stdin (replaces template body)")
}

func runNoteCreate(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind note create <path> --type <type>")
	}
	return executeNoteCreate(cmd, args[0])
}

// noteCreateResult is the JSON response payload for note create.
type noteCreateResult struct {
	Path      string   `json:"path"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Created   bool     `json:"created"`
	WriteHash string   `json:"write_hash"`
	CommitSHA string   `json:"commit_sha,omitempty"`
	Warnings  []string `json:"warnings"`
}

func executeNoteCreate(cmd *cobra.Command, notePath string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNotecreateVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNotecreateJson)
	noteType := getConfigValueWithFlags[string](cmd, "type", config.KeyAppNotecreateType)
	body := getConfigValueWithFlags[string](cmd, "body", config.KeyAppNotecreateBody)
	commit := getConfigValueWithFlags[bool](cmd, "commit", config.KeyAppNotecreateCommit)
	fieldSlice, _ := cmd.Flags().GetStringSlice("field")
	bodyStdin, _ := cmd.Flags().GetBool("body-stdin")

	if bodyStdin {
		stdinBytes, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		body = string(stdinBytes)
	}

	if noteType == "" {
		return fmt.Errorf("--type is required")
	}

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "note create")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	if !vdb.Reg.HasType(noteType) {
		return fmt.Errorf("unknown type %q (registered: %s)", noteType, strings.Join(vdb.Reg.ListTypes(), ", "))
	}

	absPath := filepath.Join(vaultPath, notePath)

	// C1: Reject paths that escape the vault directory.
	cleanVault := filepath.Clean(vaultPath)
	cleanAbs := filepath.Clean(absPath)
	if !strings.HasPrefix(cleanAbs, cleanVault+string(filepath.Separator)) && cleanAbs != cleanVault {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "note create", "path_traversal", fmt.Sprintf("path %q escapes vault", notePath))
		}
		return fmt.Errorf("path traversal: %q escapes vault directory", notePath)
	}

	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("note already exists: %s", notePath)
	}

	fields := parseFieldSlice(fieldSlice)

	td, _ := vdb.Reg.GetTypeDef(noteType)
	templatePath := filepath.Join(vaultPath, td.Template)

	result, err := tmpl.Process(tmpl.ProcessConfig{
		VaultPath:      vaultPath,
		Path:           notePath,
		Type:           noteType,
		Fields:         fields,
		Body:           body,
		TemplatePath:   templatePath,
		RequiredFields: td.Required,
	})
	if err != nil {
		return fmt.Errorf("processing template: %w", err)
	}

	// I1: Validate that type-specific required fields are present and non-empty in the processed frontmatter.
	for _, reqField := range td.Required {
		val, exists := result.FinalFrontmatter[reqField]
		empty := !exists || val == nil || val == ""
		if empty {
			return fmt.Errorf("required field %q is missing or empty; provide it with --field %s=<value>", reqField, reqField)
		}
	}

	if existing, err := vdb.DB.QueryNoteByID(result.ID); err != nil {
		return fmt.Errorf("checking ID uniqueness: %w", err)
	} else if existing != nil {
		return fmt.Errorf("ID %q already exists at %s", result.ID, existing.Path)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	if err := os.WriteFile(absPath, result.Content, 0o640); err != nil { //nolint:gosec // user-provided path within vault
		return fmt.Errorf("writing note: %w", err)
	}

	dbPath := filepath.Join(vaultPath, vdb.Config.Index.DBPath)
	if idxErr := index.NewIndexer(vaultPath, dbPath, vdb.Config).IndexFile(notePath); idxErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("re-index failed: %s", idxErr))
	}

	var commitSHA string
	if commit {
		detector := &git.GoGitDetector{}
		checker, err := git.NewPolicyChecker(vdb.Config.Git)
		if err != nil {
			return fmt.Errorf("creating policy checker: %w", err)
		}
		state, err := detector.Detect(vaultPath)
		if err != nil {
			return fmt.Errorf("detecting git state: %w", err)
		}
		if pr := checker.Check(state, git.OpWriteCommit, notePath); pr.Decision == git.Refuse {
			return fmt.Errorf("git policy refused commit: %s", pr.Reasons[0].Rule)
		}
		commitSHA, err = (&git.Committer{}).CommitFiles(vaultPath, []string{notePath}, fmt.Sprintf("feat(note): create %s", notePath))
		if err != nil {
			return fmt.Errorf("committing: %w", err)
		}
	}

	// C2: write_hash is SHA-256 of the written note content (not the DB file hash).
	h := sha256.Sum256(result.Content)
	writeHash := fmt.Sprintf("sha256:%x", h)

	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	out := noteCreateResult{
		Path:      notePath,
		ID:        result.ID,
		Type:      noteType,
		Created:   true,
		WriteHash: writeHash,
		CommitSHA: commitSHA,
		Warnings:  warnings,
	}

	if jsonOut {
		env := envelope.OK("note create", out)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created: %s (id: %s)\n", notePath, result.ID)
	return err
}

// parseFieldSlice converts ["key=value", ...] into a map.
func parseFieldSlice(fields []string) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		k, v, _ := strings.Cut(f, "=")
		if k != "" {
			m[k] = v
		}
	}
	return m
}
