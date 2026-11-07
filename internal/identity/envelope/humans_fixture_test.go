package envelope

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// TestCrossLanguageHumansFixture_ChatEnvelopes pins the human chat samples in
// the registry fixture to VerifyEnvelope. The valid sample verifies as
// human:peiman. A human prefix that claims from_class agent, an agent that
// claims from_class human, and an unknown human slug are refused.
func TestCrossLanguageHumansFixture_ChatEnvelopes(t *testing.T) {
	raw, err := os.ReadFile("../registry/testdata/cross_language_humans_vectors.json")
	require.NoError(t, err)
	var fx struct {
		RootPubKey      string `json:"root_pubkey"`
		Now             int64  `json:"now"`
		MaxStalenessSec int64  `json:"max_staleness_secs"`
		Valid           struct {
			Envelope       json.RawMessage `json:"envelope"`
			PersistedEpoch int             `json:"persisted_highest_epoch"`
		} `json:"valid"`
		Envelopes struct {
			Valid  humansFixtureChat   `json:"valid"`
			Reject []humansFixtureChat `json:"reject"`
		} `json:"envelopes"`
	}
	require.NoError(t, json.Unmarshal(raw, &fx))

	parsed, err := registry.ParseDistribution(fx.Valid.Envelope)
	require.NoError(t, err)
	rootPub, err := base64.StdEncoding.DecodeString(fx.RootPubKey)
	require.NoError(t, err)
	reg, _, err := registry.VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err)

	assertHumanChat(t, reg, fx.Envelopes.Valid, fx.Now, true)
	require.Len(t, fx.Envelopes.Reject, 3)
	for _, chat := range fx.Envelopes.Reject {
		t.Run(chat.Name, func(t *testing.T) {
			assertHumanChat(t, reg, chat, fx.Now, false)
		})
	}
}

type humansFixtureChat struct {
	Name      string `json:"name"`
	Expect    string `json:"expect"`
	Body      string `json:"body"`
	FromAgent string `json:"from_agent"`
	FromClass string `json:"from_class"`
	KeyEpoch  int64  `json:"key_epoch"`
	Kind      string `json:"kind"`
	Nonce     string `json:"nonce"`
	Room      string `json:"room"`
	Seq       int64  `json:"seq"`
	TS        int64  `json:"ts"`
	Canonical string `json:"canonical"`
	Signature string `json:"signature"`
}

func assertHumanChat(t *testing.T, reg registry.Registry, chat humansFixtureChat, now int64, accept bool) {
	t.Helper()
	fields := Fields{
		AlgVersion: AlgVersion,
		Body:       chat.Body,
		FromAgent:  chat.FromAgent,
		FromClass:  strptr(chat.FromClass),
		KeyEpoch:   chat.KeyEpoch,
		Kind:       strptr(chat.Kind),
		Nonce:      chat.Nonce,
		Room:       strptr(chat.Room),
		Seq:        chat.Seq,
		TS:         chat.TS,
	}
	canonical, err := CanonicalizeEnvelope(fields)
	require.NoError(t, err)
	assert.Equal(t, chat.Canonical, string(canonical.Bytes()))
	sig, err := base64.StdEncoding.DecodeString(chat.Signature)
	require.NoError(t, err)
	ok, err := VerifyEnvelope(reg, fields, sig, time.Unix(now, 0))
	if accept {
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "accept", chat.Expect)
		return
	}
	require.Error(t, err)
	assert.False(t, ok)
	assert.Equal(t, "reject", chat.Expect)
}
