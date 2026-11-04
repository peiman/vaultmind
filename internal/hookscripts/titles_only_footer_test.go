package hookscripts_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A ranked title list must not be described as note text (#409). Exercise
// both hooks under stock bash, including the read command's vault selection.
func TestRecallHooks_FooterMatchesDeliveredContent(t *testing.T) {
	const titles = "Search: retries\n  0.42  x   Retry policy\n  0.31  y   Retry limits\n  0.20  z   Retry timing\nContext from: x — 3 notes, 0 delivered\n"
	cases := []struct {
		name       string
		askOutput  string
		titlesOnly bool
	}{
		{"confidence withheld", titles + "(pointers only: 220 tokens assembled and withheld — the top hit's confidence is below the delivery floor)\n", true},
		{"budget withheld", titles + "(pointers only: no body fits the budget)\n", true},
		{"deduplicated", titles + "(pointers only — all bodies were recently delivered)\n", true},
		{"delivered excerpt", "Search: retries\n  0.42  x   Retry policy\nContext from: x — 1 note, 1 delivered as excerpt (10 tok)\nUse bounded retries.\n", false},
		{"marker inside delivered text", "Context from: x — 1 note, 1 delivered as excerpt (10 tok)\nThe marker (pointers only is discussed here.\n", false},
	}
	for _, hook := range []string{"vault-reach.sh", "vault-recall.sh"} {
		for _, federated := range []bool{false, true} {
			mode := "single vault"
			if federated {
				mode = "federated"
			}
			for _, tc := range cases {
				t.Run(hook+"/"+mode+"/"+tc.name, func(t *testing.T) {
					stub := "if [ \"$1\" = ask ]; then\ncat <<'ASK_OUTPUT'\n" + tc.askOutput + "ASK_OUTPUT\nfi\nexit 0\n"
					h := newHookEnv(t, stub)
					env := h.env(false)
					where := "--vault " + h.projectDir + "/vaultmind-identity"
					if federated {
						where = "--vaults /vaults/alpha,/vaults/beta"
						env = append(env, "VAULTMIND_VAULTS=/vaults/alpha,/vaults/beta")
					}
					payload := `{"prompt":"what did we decide about retries?","session_id":"test"}`
					if hook == "vault-reach.sh" {
						payload = bashPayload(`git commit -m "fix: retry policy"`)
					}
					out, _ := runHookScript(t, hook, env, payload)
					if hook == "vault-reach.sh" {
						var response struct {
							HookSpecificOutput struct {
								AdditionalContext string `json:"additionalContext"`
							} `json:"hookSpecificOutput"`
						}
						require.NoError(t, json.Unmarshal([]byte(out), &response))
						out = response.HookSpecificOutput.AdditionalContext
					}
					assert.Contains(t, out, tc.askOutput)
					if tc.titlesOnly {
						assert.Contains(t, out, "Titles only: no note text was delivered above. Read one with: vaultmind note get <id> "+where)
						assert.NotContains(t, out, "the note's own text")
					} else {
						footer := "(the note's own text — a Principle section where it has one, else its opening lines. Full note: vaultmind note get <id> " + where + ")\n"
						if hook == "vault-reach.sh" {
							footer = "\nThat is the note's own text, not a pointer to it: a Principle section where it has one, else its opening lines, which may be context rather than a rule. Act on it if it bears on this call — full note: vaultmind note get <id> " + where + "."
						}
						assert.Contains(t, out, footer)
						assert.NotContains(t, out, "Titles only")
					}
				})
			}
		}
	}
}
