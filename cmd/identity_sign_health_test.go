package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/signhealth"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSignHealthRecord(t *testing.T, agent string, record string) string {
	t.Helper()
	path, err := xdg.StateFile(filepath.Join("sign-health", signhealth.SanitizeAgent(agent)+".json"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(record), 0o600))
	return path
}

func TestIdentitySignHealth_ExitsOneAndNamesFailingAgent(t *testing.T) {
	isolateSignHealth(t)
	writeSignHealthRecord(t, "agent:cm", `{"agent":"agent:cm","ok":false,"ts":"2026-11-19T15:32:47Z","error":"registry failed","version":"v1"}`)

	out, _, err := runRootCmd(t, "identity", "sign-health")
	require.EqualError(t, err, "1 agent(s) cannot sign: agent:cm")
	assert.Contains(t, out.String(), "agent:cm  FAIL  2026-11-19T15:32:47Z  registry failed")
}

func TestIdentitySignHealth_CorruptFileIsUnreadableAndFails(t *testing.T) {
	isolateSignHealth(t)
	path := writeSignHealthRecord(t, "agent:cm", "not json")

	out, _, err := runRootCmd(t, "identity", "sign-health")
	require.Error(t, err)
	assert.Contains(t, out.String(), "UNREADABLE "+path+":")
}

func TestIdentitySignHealth_MaxAgeMakesOldFailureStale(t *testing.T) {
	isolateSignHealth(t)
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	writeSignHealthRecord(t, "agent:cm", `{"agent":"agent:cm","ok":false,"ts":"`+old+`","error":"old failure","version":"v1"}`)

	out, _, err := runRootCmd(t, "identity", "sign-health", "--max-age", "1h")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "agent:cm  stale  "+old+"  old failure")
}

func TestIdentitySignHealth_JSONReportsAgents(t *testing.T) {
	isolateSignHealth(t)
	writeSignHealthRecord(t, "agent:mira", `{"agent":"agent:mira","ok":true,"ts":"2026-11-19T15:32:47Z","version":"v1"}`)

	out, _, err := runRootCmd(t, "identity", "sign-health", "--output-format", "json")
	require.NoError(t, err)
	var got struct {
		Agents []map[string]any `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got.Agents, 1)
	assert.Equal(t, "agent:mira", got.Agents[0]["agent"])
	assert.Equal(t, true, got.Agents[0]["ok"])
}
