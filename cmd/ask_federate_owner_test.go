package cmd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingRetriever counts Search calls. The federated owner used to be
// searched twice: once to pick it, once inside Ask to pack and record access.
type countingRetriever struct {
	calls int
	hit   retrieval.ScoredResult
}

func (c *countingRetriever) Search(context.Context, string, int, int, index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	c.calls++
	if c.hit.ID == "" {
		return nil, 0, nil
	}
	return []retrieval.ScoredResult{c.hit}, 1, nil
}

// countingEmbedder counts cache-miss forwards. Embed of the same text is one
// forward: production BGE-M3 memoizes on the embedder instance, and both the
// verdict cosine and the delivery similarities reuse that vector.
type countingEmbedder struct {
	forwards int
	memo     map[string][]float32
}

func (e *countingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if e.memo == nil {
		e.memo = map[string][]float32{}
	}
	if v, ok := e.memo[text]; ok {
		return v, nil
	}
	e.forwards++
	v := []float32{1, 0, 0}
	e.memo[text] = v
	return v, nil
}

func (e *countingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v, err := e.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (e *countingEmbedder) Dims() int { return 3 }

func (e *countingEmbedder) Close() error { return nil }

// AnalyzeQuery goes through Embed so the memo key is the shaped prefix.
// Empty weights leave salient selection off; the forward bound still applies.
func (e *countingEmbedder) AnalyzeQuery(ctx context.Context, text string) (embedding.QueryTokens, error) {
	_, err := e.Embed(ctx, text)
	return embedding.QueryTokens{}, err
}

type vaultProbe struct {
	ret *countingRetriever
	emb *countingEmbedder
}

func installCountingOpener(t *testing.T, hit retrieval.ScoredResult) map[string]*vaultProbe {
	t.Helper()
	prev := openAskIndex
	probes := map[string]*vaultProbe{}
	openAskIndex = func(_ *cobra.Command, vaultPath string, _ *index.DB) query.AutoRetrieverResult {
		if p, ok := probes[vaultPath]; ok {
			return query.AutoRetrieverResult{Retriever: p.ret, Embedder: p.emb, Cleanup: func() {}}
		}
		p := &vaultProbe{
			ret: &countingRetriever{hit: hit},
			emb: &countingEmbedder{},
		}
		probes[vaultPath] = p
		return query.AutoRetrieverResult{Retriever: p.ret, Embedder: p.emb, Cleanup: func() {}}
	}
	t.Cleanup(func() { openAskIndex = prev })
	return probes
}

func copyNamedVaults(t *testing.T, names []string) []string {
	t.Helper()
	src := testvault.IndexedFixtureVault(t)
	parent := t.TempDir()
	paths := make([]string, len(names))
	for i, name := range names {
		dst := filepath.Join(parent, name)
		require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
		paths[i] = dst
	}
	return paths
}

func restoreAskViper(t *testing.T, keys ...string) {
	t.Helper()
	saved := make(map[string]any, len(keys))
	for _, k := range keys {
		saved[k] = viper.Get(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			viper.Set(k, v)
		}
	})
}

// One search per vault, and the owner's embedder forwards once.
//
// federateAndPickOwner already searches the owner. runAsk used to open it
// again and call query.Ask, which searches it a second time — 4 searches for
// 3 vaults — only to pack context and record access. The second search is
// what this count goes red on. The forward count stays at one when delivery
// reuses the shaped query on the same memoizing embedder, and goes red when
// delivery embeds a different string.
func TestAsk_FederatedOwnerIsSearchedOnce(t *testing.T) {
	longQuery := strings.TrimSpace(strings.Repeat("spreading activation memory consolidation ", 12))
	tests := []struct {
		name    string
		vaults  []string
		query   string
		forward int
	}{
		{name: "two vaults", vaults: []string{"a-desk", "b-research"}, query: "spreading activation"},
		{name: "three vaults", vaults: []string{"a-desk", "b-mine", "c-research"}, query: "spreading activation"},
		{name: "shaped prefix is embedded once", vaults: []string{"a-desk", "b-research"}, query: longQuery, forward: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreAskViper(t, config.KeyAppAskVaults, config.KeyAppAskQueryForwardTokens)
			paths := copyNamedVaults(t, tt.vaults)
			probes := installCountingOpener(t, retrieval.ScoredResult{
				ID:    "concept-spreading-activation",
				Type:  "concept",
				Title: "Spreading Activation",
				Path:  "concepts/spreading-activation.md",
				Score: 1,
			})

			cmd := MustNewCommand(commands.AskMetadata, runAsk)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.Flags().Set("vaults", strings.Join(paths, ",")))
			if tt.forward > 0 {
				require.NoError(t, cmd.Flags().Set("query-forward-tokens", strconv.Itoa(tt.forward)))
			}

			require.NoError(t, runAsk(cmd, []string{tt.query}))

			owner := paths[0]
			for _, p := range paths[1:] {
				if filepath.Base(p) < filepath.Base(owner) {
					owner = p
				}
			}
			for _, path := range paths {
				probe := probes[path]
				require.NotNil(t, probe, "opener must have built %s", filepath.Base(path))
				assert.Equal(t, 1, probe.ret.calls, "vault %s searches", filepath.Base(path))
			}
			assert.Equal(t, 1, probes[owner].emb.forwards, "owner %s embedder forwards", filepath.Base(owner))
		})
	}
}

// Output of a fixture federation, blanking only the fields that move between
// runs: envelope time, index hash, and vault path. Hits, order, context pack,
// and access rows stay.
func TestAsk_FederatedOwnerOutputMatchesCurrent(t *testing.T) {
	tests := []struct {
		name   string
		json   bool
		stdout string
		access string
	}{
		{name: "json", json: true, stdout: "json.golden", access: "access-json.golden"},
		{name: "text", json: false, stdout: "text.golden", access: "access-text.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreAskViper(t, config.KeyAppAskVaults, config.KeyAppAskJson, config.KeyAppAskMaxItems, config.KeyAppAskBudget, config.KeyAppAskExcerpt)
			paths := copyNamedVaults(t, []string{"desk", "research"})
			stdout, access := runFixtureFederation(t, paths, tt.json)
			assert.Equal(t, readGolden(t, tt.stdout), blankFederatedOutput(t, stdout, tt.json, paths))
			assert.Equal(t, readGolden(t, tt.access), access)
		})
	}
}

func runFixtureFederation(t *testing.T, paths []string, jsonOut bool) (string, string) {
	t.Helper()
	cmd := MustNewCommand(commands.AskMetadata, runAsk)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("vaults", strings.Join(paths, ",")))
	require.NoError(t, cmd.Flags().Set("max-items", "3"))
	require.NoError(t, cmd.Flags().Set("budget", "1500"))
	require.NoError(t, cmd.Flags().Set("excerpt", "80"))
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	require.NoError(t, runAsk(cmd, []string{"spreading activation"}))
	return out.String(), ownerAccessRows(t, paths[0])
}

func ownerAccessRows(t *testing.T, owner string) string {
	t.Helper()
	vdb, err := cmdutil.OpenVaultDB(owner)
	require.NoError(t, err)
	t.Cleanup(vdb.Close)
	rows, err := vdb.DB.Query(`SELECT note_id, caller, body_delivered FROM note_accesses ORDER BY note_id, caller, rowid`)
	require.NoError(t, err)
	defer rows.Close()
	type accessRow struct {
		NoteID        string `json:"note_id"`
		Caller        string `json:"caller"`
		BodyDelivered *bool  `json:"body_delivered"`
	}
	var got []accessRow
	for rows.Next() {
		var row accessRow
		var delivered sql.NullBool
		require.NoError(t, rows.Scan(&row.NoteID, &row.Caller, &delivered))
		if delivered.Valid {
			v := delivered.Bool
			row.BodyDelivered = &v
		}
		got = append(got, row)
	}
	require.NoError(t, rows.Err())
	buf, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	return string(buf) + "\n"
}

func blankFederatedOutput(t *testing.T, raw string, jsonOut bool, paths []string) string {
	t.Helper()
	if !jsonOut {
		for _, p := range paths {
			raw = strings.ReplaceAll(raw, p, "VAULT")
		}
		return raw
	}
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	if meta, ok := env["meta"].(map[string]any); ok {
		meta["timestamp"] = ""
		meta["vault_path"] = ""
		meta["index_hash"] = ""
	}
	buf, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	return string(buf) + "\n"
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "federated-owner-once", name))
	require.NoError(t, err)
	return string(b)
}
