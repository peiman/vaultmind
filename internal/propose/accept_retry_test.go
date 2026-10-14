package propose

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccept_RetryAfterIndexDeleteFailure(t *testing.T) {
	v, req, out := readyPatch(t)
	var calls int
	req.deleteIndexed = func(db *index.DB, rel string) error {
		calls++
		if calls == 1 {
			return errors.New("index busy")
		}
		return index.DeleteNoteByPath(db, rel)
	}

	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index")
	_, statErr := os.Stat(filepath.Join(v.Path, "hold", "proposal-quartz.md"))
	assert.NoError(t, statErr, "a failed index delete must leave the proposal file")
	row, err := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, err)
	assert.NotNil(t, row)
	assert.Equal(t, 1, strings.Count(targetText(t, v), acceptedMarker("proposal-quartz")))

	require.NoError(t, Accept(context.Background(), v, req))
	assert.Equal(t, 1, strings.Count(targetText(t, v), acceptedMarker("proposal-quartz")))
	assert.Equal(t, 1, strings.Count(targetText(t, v), "## How does quartz work?"))
	_, statErr = os.Stat(filepath.Join(v.Path, "hold", "proposal-quartz.md"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	gone, err := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Contains(t, out.String(), "accepted concept-memory")
	assert.Contains(t, out.String(), "removed hold/proposal-quartz.md")
}

func TestAccept_RetryAfterFileDeleteFailure(t *testing.T) {
	v, req, _ := readyPatch(t)
	var calls int
	req.removeFile = func(path string) error {
		calls++
		if calls == 1 {
			return errors.New("file busy")
		}
		return os.Remove(path)
	}

	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "removing proposal")
	_, statErr := os.Stat(filepath.Join(v.Path, "hold", "proposal-quartz.md"))
	assert.NoError(t, statErr)
	gone, err := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, err)
	assert.Nil(t, gone, "the index row is removed before the file")
	assert.Equal(t, 1, strings.Count(targetText(t, v), acceptedMarker("proposal-quartz")))

	require.NoError(t, Accept(context.Background(), v, req))
	assert.Equal(t, 1, strings.Count(targetText(t, v), acceptedMarker("proposal-quartz")))
	assert.Equal(t, 1, strings.Count(targetText(t, v), "## How does quartz work?"))
	_, statErr = os.Stat(filepath.Join(v.Path, "hold", "proposal-quartz.md"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	gone, err = v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func TestAccept_ProposalFileAlreadyGoneRemovesTheRow(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		v, req, out := readyPatch(t)
		require.NoError(t, os.Remove(filepath.Join(v.Path, "hold", "proposal-quartz.md")))
		require.NoError(t, Accept(context.Background(), v, req))
		assert.Contains(t, out.String(), "already gone")
		assert.Contains(t, out.String(), "proposal-quartz")
		gone, err := v.DB.QueryNoteByID("proposal-quartz")
		require.NoError(t, err)
		assert.Nil(t, gone)
		assert.NotContains(t, targetText(t, v), "Quartz keeps one timeline.")
	})

	t.Run("json", func(t *testing.T) {
		v, req, out := readyPatch(t)
		req.JSON = true
		require.NoError(t, os.Remove(filepath.Join(v.Path, "hold", "proposal-quartz.md")))
		require.NoError(t, Accept(context.Background(), v, req))
		var env struct {
			Status string `json:"status"`
			Result struct {
				Mode string `json:"mode"`
				ID   string `json:"id"`
				Path string `json:"path"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "ok", env.Status)
		assert.Equal(t, "cleanup", env.Result.Mode)
		assert.Equal(t, "proposal-quartz", env.Result.ID)
		assert.Equal(t, "hold/proposal-quartz.md", env.Result.Path)
	})
}

func TestAccept_MissingFileThatIsNotAProposalStaysIndexed(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "draft", "The original body.")
	require.NoError(t, os.Remove(filepath.Join(v.Path, "concepts", "memory.md")))
	req, _, _, _ := reqOf("", "")
	req.ID = "concept-memory"
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
	row, qerr := v.DB.QueryNoteByID("concept-memory")
	require.NoError(t, qerr)
	assert.NotNil(t, row)
}

func TestAccept_UnindexedPublishedNoteIsNotRevived(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The original body.")
	require.NoError(t, indexDelete(t, v, "concepts/memory.md"))
	req, _, _, _ := reqOf("", "")
	req.ID = "concept-memory"
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
	_, statErr := os.Stat(filepath.Join(v.Path, "concepts", "memory.md"))
	assert.NoError(t, statErr)
}

func TestAccept_DeleteAlreadyGoneStillRemovesTheFile(t *testing.T) {
	v, req, out := readyPatch(t)
	req.deleteIndexed = func(*index.DB, string) error { return sql.ErrNoRows }
	require.NoError(t, Accept(context.Background(), v, req))
	assert.Contains(t, out.String(), "accepted concept-memory")
	_, statErr := os.Stat(filepath.Join(v.Path, "hold", "proposal-quartz.md"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	assert.Equal(t, 1, strings.Count(targetText(t, v), acceptedMarker("proposal-quartz")))
}

func TestAccept_UnreadableProposalIsNotRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode 000")
	}
	v, req, _ := readyPatch(t)
	path := filepath.Join(v.Path, "hold", "proposal-quartz.md")
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := os.ReadFile(path); err == nil || os.IsNotExist(err) {
		t.Skip("mode 000 did not block the read")
	}
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
	row, qerr := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, qerr)
	assert.NotNil(t, row)
}

func TestAccept_BadPatchesValueIsNotAProposal(t *testing.T) {
	v, req, _ := readyPatch(t)
	require.NoError(t, os.Remove(filepath.Join(v.Path, "hold", "proposal-quartz.md")))
	res, err := v.DB.Exec(`UPDATE frontmatter_kv SET value_json = ? WHERE note_id = ? AND key = 'patches'`, "1", "proposal-quartz")
	require.NoError(t, err)
	n, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	err = Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
	row, qerr := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, qerr)
	assert.NotNil(t, row)
}

func TestAccept_RowDeleteFailureWhenTheFileIsGone(t *testing.T) {
	v, req, _ := readyPatch(t)
	require.NoError(t, os.Remove(filepath.Join(v.Path, "hold", "proposal-quartz.md")))
	req.deleteIndexed = func(*index.DB, string) error { return errors.New("index busy") }
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index")
	row, qerr := v.DB.QueryNoteByID("proposal-quartz")
	require.NoError(t, qerr)
	assert.NotNil(t, row)
}

func TestReindexUnindexedProposal_Edges(t *testing.T) {
	v := testVault(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reindexUnindexedProposal(ctx, v, "missing")
	require.ErrorIs(t, err, context.Canceled)

	bare := v
	bare.Config = nil
	row, err := reindexUnindexedProposal(context.Background(), bare, "missing")
	require.NoError(t, err)
	assert.Nil(t, row)

	gone := v
	gone.Path = filepath.Join(v.Path, "no-such-vault")
	_, err = reindexUnindexedProposal(context.Background(), gone, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "looking for proposal")

	row, err = reindexUnindexedProposal(context.Background(), v, "not-a-note")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func readyPatch(t *testing.T) (Vault, Request, *bytes.Buffer) {
	t.Helper()
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The original body.")
	writeRawNote(t, v, "hold/proposal-quartz.md", proposalNote("proposal-quartz", "concept-memory", "How does quartz work?", "Quartz keeps one timeline.", nil))
	req, out, _, _ := reqOf("", "")
	req.ID = "proposal-quartz"
	return v, req, out
}

func targetText(t *testing.T, v Vault) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(v.Path, "concepts", "memory.md"))
	require.NoError(t, err)
	return string(body)
}
