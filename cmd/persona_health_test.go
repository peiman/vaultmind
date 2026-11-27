package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/personahealth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func personaHealthFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "internal", "personahealth", "testdata", name)
}

func isolatePersonaHealth(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cleanupCLIState(t)
}

func TestPersonaHealth_TextReportsKeeperAndFailsTitleOnly(t *testing.T) {
	isolatePersonaHealth(t)
	out, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/title-only.jsonl"),
		"--vault", personaHealthFixture(t, "vault-title"), "--name", "keeper")

	require.Error(t, err)
	assert.Contains(t, out.String(), "keeper  FAIL  identity 0%")
	assert.Contains(t, out.String(), "arcs 0 full / 1 partial / 0 missing")
}

func TestPersonaHealth_TextUsesExplicitKeeperName(t *testing.T) {
	isolatePersonaHealth(t)
	out, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--name", "mira")

	require.NoError(t, err)
	assert.Contains(t, out.String(), "mira  OK")
	assert.NotContains(t, out.String(), "arc arc-boundary body")
}

func TestPersonaHealth_JSONUsesStableEnvelopeAndKeeperFields(t *testing.T) {
	isolatePersonaHealth(t)
	out, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--name", "keeper", "--json")

	require.NoError(t, err)
	var got struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			Keepers []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"keepers"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "v1", got.SchemaVersion)
	assert.Equal(t, "ok", got.Status)
	require.Len(t, got.Result.Keepers, 1)
	assert.Equal(t, "keeper", got.Result.Keepers[0].Name)
	assert.Equal(t, "ok", got.Result.Keepers[0].Status)

	_, _, err = runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/title-only.jsonl"),
		"--vault", personaHealthFixture(t, "vault-title"), "--json")
	assert.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
}

func TestPersonaHealth_JSONIncludesArcBodyItemsAndTotals(t *testing.T) {
	isolatePersonaHealth(t)
	out, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/arc-body.jsonl"),
		"--vault", personaHealthFixture(t, "vault-body"), "--json")

	require.NoError(t, err)
	var got struct {
		Result struct {
			Keepers []struct {
				Arcs struct {
					Full         int     `json:"full"`
					BodyBytes    int     `json:"body_bytes"`
					ArrivedBytes int     `json:"arrived_bytes"`
					Coverage     float64 `json:"coverage"`
					Items        []struct {
						ID           string `json:"id"`
						BodyBytes    int    `json:"body_bytes"`
						ArrivedBytes int    `json:"arrived_bytes"`
					} `json:"items"`
				} `json:"arcs"`
			} `json:"keepers"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got.Result.Keepers, 1)
	arcs := got.Result.Keepers[0].Arcs
	assert.Equal(t, 1, arcs.Full)
	assert.Equal(t, 78, arcs.BodyBytes)
	assert.Equal(t, 26, arcs.ArrivedBytes)
	assert.InDelta(t, 26.0/78.0, arcs.Coverage, 0.0001)
	require.Len(t, arcs.Items, 2)
	assert.Equal(t, "arc-first", arcs.Items[0].ID)
	assert.Equal(t, 26, arcs.Items[0].ArrivedBytes)
	assert.Equal(t, 0, arcs.Items[1].ArrivedBytes)
}

func TestPersonaHealth_JSONIncludesArcMomentSectionsAndTotal(t *testing.T) {
	isolatePersonaHealth(t)
	out, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--json")
	require.NoError(t, err)
	var got struct {
		Result struct {
			Keepers []struct {
				Arcs struct {
					MomentCoverage float64 `json:"moment_coverage"`
					MomentBytes    int     `json:"moment_bytes"`
					Items          []struct {
						Sections []struct {
							Name         string  `json:"name"`
							Bytes        int     `json:"bytes"`
							ArrivedBytes int     `json:"arrived_bytes"`
							Coverage     float64 `json:"coverage"`
						} `json:"sections"`
					} `json:"items"`
				} `json:"arcs"`
			} `json:"keepers"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got.Result.Keepers, 1)
	arcs := got.Result.Keepers[0].Arcs
	assert.Greater(t, arcs.MomentBytes, 0)
	assert.Greater(t, arcs.MomentCoverage, 0.0)
	require.Len(t, arcs.Items, 1)
	assert.Len(t, arcs.Items[0].Sections, 2)
}

func TestPersonaHealth_RejectsBadMinimumAndMixedForms(t *testing.T) {
	isolatePersonaHealth(t)
	_, _, err := runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--min-coverage", "1.1")
	assert.EqualError(t, err, "--min-coverage must be between 0 and 1")

	_, _, err = runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--min-arc-coverage", "1.1")
	assert.EqualError(t, err, "--min-arc-coverage must be between 0 and 1")

	_, _, err = runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--min-moment-coverage", "1.1")
	assert.EqualError(t, err, "--min-moment-coverage must be between 0 and 1")

	_, _, err = runRootCmd(t, "persona", "health",
		"--transcripts", personaHealthFixture(t, "transcripts/full.jsonl"),
		"--vault", personaHealthFixture(t, "vault-full"), "--keepers", filepath.Join(t.TempDir(), "keepers.yaml"))
	assert.Error(t, err)
}

type personaHealthFailWriter struct{}

func (personaHealthFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type personaHealthWriterCommand struct{}

func (personaHealthWriterCommand) OutOrStdout() io.Writer { return personaHealthFailWriter{} }

func TestPersonaHealth_JSONWriteFailureIsReturned(t *testing.T) {
	err := renderPersonaHealth(personaHealthWriterCommand{}, personahealth.Result{}, true)
	assert.EqualError(t, err, "write failed")
}
