package cmd

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This section prints on every run with data, unlike every other doctor
// section, which is an exception report. "Nobody is reading this vault"
// produces no exception anywhere — retrieval succeeds, doctor is green, and the
// memory goes unused for months. Silence is exactly how that state hides.
func TestWriteMemoryUse_PrintsEvenWhenNothingIsWrong(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMemoryUse(&buf, &query.DoctorMemoryUse{
		WindowDays: 7, Injections: 100, BodiesDelivered: 100, Consumed: 60,
		NotesBanked: 3, VaultNotes: 63,
	}))
	out := buf.String()
	assert.Contains(t, out, "100 surfaced")
	assert.Contains(t, out, "60 read")
	assert.Contains(t, out, "60.0%")
	assert.Contains(t, out, "banked")
	assert.NotContains(t, out, "⚠", "a healthy rollup carries no warning")
}

// The distinction that decides what to fix. Without it, a reader sees 0.8% and
// concludes the agent ignores its memory — when the tool never handed it over.
func TestWriteMemoryUse_NamesWithholdingWhenNoBodiesWereDelivered(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMemoryUse(&buf, &query.DoctorMemoryUse{
		WindowDays: 7, Injections: 6215, BodiesDelivered: 0, Consumed: 47,
	}))
	out := buf.String()
	assert.Contains(t, out, "no bodies delivered")
	assert.Contains(t, out, "withholding")
}

// A fresh install has no usage log. It must not be told its memory is ignored —
// that is the false positive that gets a signal switched off.
func TestWriteMemoryUse_SilentWithoutData(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMemoryUse(&buf, nil))
	assert.Empty(t, buf.String(), "no usage log means not measured, not unhealthy")

	buf.Reset()
	require.NoError(t, writeMemoryUse(&buf, &query.DoctorMemoryUse{WindowDays: 7}))
	assert.Empty(t, buf.String(), "nothing surfaced yet is not a finding")
}

// Each hook gets its own verdict: session-start, mid-task and compaction are
// different bets, and an aggregate lets a good one hide inside a bad one.
func TestWriteMemoryUse_SplitsPerHook(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMemoryUse(&buf, &query.DoctorMemoryUse{
		WindowDays: 7, Injections: 10, Consumed: 1,
		PerCaller: []query.DoctorCallerUse{
			{Caller: "vaultmind-reach-hook", Injections: 6, Consumed: 1},
			{Caller: "vaultmind-persona-hook", Injections: 4, Consumed: 0},
		},
	}))
	out := buf.String()
	assert.Contains(t, out, "vaultmind-reach-hook")
	assert.Contains(t, out, "vaultmind-persona-hook")
}

func TestWriteMemoryUse_PropagatesWriteErrors(t *testing.T) {
	m := &query.DoctorMemoryUse{
		WindowDays: 7, Injections: 10, BodiesDelivered: 0, Consumed: 1,
		PerCaller: []query.DoctorCallerUse{{Caller: "h", Injections: 10, Consumed: 1}},
	}
	for ok := 0; ok < 4; ok++ {
		require.Error(t, writeMemoryUse(&failAfterNWriter{ok: ok}, m),
			"write failure at position %d must propagate", ok)
	}
}
