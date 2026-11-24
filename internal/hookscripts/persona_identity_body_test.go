package hookscripts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// relevanceGateStub reproduces the identity query's weak-hit behavior: the
// menu is present without an explicit read, but the note body is withheld.
const relevanceGateStub = `#!/bin/bash
case "$1" in
  ask)
    if [ "$2" = "who am I" ]; then
      for arg in "$@"; do
        if [ "$arg" = "1" ]; then
          echo 'Search: "who am I" (5 hits)'
          echo 'WHO-I-AM-BODY-TEXT'
          exit 0
        fi
      done
      echo 'Search: "who am I" (5 hits) [relevance: weak — body suppressed]'
      echo 'Context from: identity-who-i-am — 9 notes, 0 delivered'
    else
      echo 'CURRENT-CONTEXT-POINTERS'
    fi
    ;;
  arc) echo 'ARCS'
    ;;
  self) echo 'SELF'
    ;;
esac
exit 0
`

func TestPersonaHook_DeliversIdentityBodyThroughWeakRelevanceGate(t *testing.T) {
	project, env := installPersonaHook(t, relevanceGateStub)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	out := runPersonaHookSession(t, project, env, "s-identity-body")

	assert.Contains(t, out, "WHO-I-AM-BODY-TEXT")
	rec := lastSidecar(t, env)
	assert.Equal(t, false, rec["identity_body_withheld"])
}

func TestPersonaHook_SidecarRecordsWithheldIdentityBody(t *testing.T) {
	project, env := installPersonaHook(t, `#!/bin/bash
if [ "$1" = "ask" ] && [ "$2" = "who am I" ]; then
  echo 'Search: "who am I" [body suppressed]'
elif [ "$1" = "ask" ]; then
  echo 'CURRENT-CONTEXT-POINTERS'
elif [ "$1" = "arc" ]; then
  echo 'ARCS'
elif [ "$1" = "self" ]; then
  echo 'SELF'
fi
`)
	env = append(env, "VAULTMIND_PERSONA_MODE=served")

	runPersonaHookSession(t, project, env, "s-identity-withheld")

	rec := lastSidecar(t, env)
	assert.Equal(t, true, rec["identity_body_withheld"])
}
