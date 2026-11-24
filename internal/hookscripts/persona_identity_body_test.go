package hookscripts_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// identityAskStub models the two identity asks. The gate-modelling distinction
// is the presence of --read itself: the value is deliberately irrelevant.
func identityAskStub(mode string) string {
	return fmt.Sprintf(`#!/bin/bash
has_read=0
for arg in "$@"; do
  [ "$arg" = "--read" ] && has_read=1
done
fill() { head -c "$1" /dev/zero | tr '\0' "$2"; echo; }
if [ "$1" = "ask" ] && [ "$2" = "who am I" ]; then
  if [ "$has_read" = "1" ]; then
    echo 'Search: "who am I" (5 hits)'
    if [ "%s" = size ]; then fill 7000 B; fi
    echo 'WHO-I-AM-BODY-TEXT'
  elif [ "%s" = weak ]; then
    echo 'Search: "who am I" (5 hits) [relevance: weak — body suppressed]'
    echo 'Context from: identity-who-i-am — 9 notes, 0 delivered'
  elif [ "%s" = ten ]; then
    echo 'Search: "who am I" (12 hits)'
    echo 'Context from: identity-who-i-am — 12 notes, 10 delivered as excerpts (900 tok)'
    echo 'PACK-EXCERPT-TEXT'
  else
    echo 'Search: "who am I" (5 hits)'
    echo 'Context from: identity-who-i-am — 9 notes, 9 delivered as excerpts (558 tok)'
    if [ "%s" = size ]; then fill 3000 P; fi
    echo 'PACK-EXCERPT-TEXT'
  fi
elif [ "$1" = ask ] || [ "$1" = self ] || [ "$1" = arc ]; then
  :
fi
exit 0
`, mode, mode, mode, mode)
}

func runIdentityCase(t *testing.T, mode string) (string, map[string]any) {
	t.Helper()
	project, env := installPersonaHook(t, identityAskStub(mode))
	env = append(env, "VAULTMIND_PERSONA_MODE=served")
	return runPersonaHookSession(t, project, env, "s-identity-"+mode), lastSidecar(t, env)
}

func TestPersonaHook_WeakIdentityAlwaysDeliversBodyAndDropsPack(t *testing.T) {
	out, rec := runIdentityCase(t, "weak")

	assert.Contains(t, out, "WHO-I-AM-BODY-TEXT")
	assert.NotContains(t, out, "Context from:")
	assert.Equal(t, true, rec["identity_body_withheld"])
	assert.Equal(t, false, rec["identity_pack_kept"])
}

func TestPersonaHook_StrongIdentityDeliversBodyAndPack(t *testing.T) {
	out, rec := runIdentityCase(t, "strong")

	assert.Contains(t, out, "WHO-I-AM-BODY-TEXT")
	assert.Contains(t, out, "PACK-EXCERPT-TEXT")
	assert.Equal(t, false, rec["identity_body_withheld"])
	assert.Equal(t, true, rec["identity_pack_kept"])
}

func TestPersonaHook_IdentityBodyWinsWhenBodyAndPackExceedCap(t *testing.T) {
	out, rec := runIdentityCase(t, "size")

	assert.Contains(t, out, "WHO-I-AM-BODY-TEXT")
	assert.NotContains(t, out, "PACK-EXCERPT-TEXT")
	assert.Contains(t, out, "Left out to stay under the hook delivery limit:")
	assert.Contains(t, out, "the identity context pack")
	assert.LessOrEqual(t, len(out), deliveryCap, "hook output cap is measured in bytes")
	assert.Equal(t, false, rec["identity_pack_kept"])
}

// "10 delivered" must not read as "0 delivered": a pack that delivered ten or
// more items is kept, like any other pack that delivered something.
func TestPersonaHook_PackWithTenDeliveredIsKept(t *testing.T) {
	out, rec := runIdentityCase(t, "ten")

	assert.Contains(t, out, "WHO-I-AM-BODY-TEXT")
	assert.Contains(t, out, "PACK-EXCERPT-TEXT")
	assert.Equal(t, true, rec["identity_pack_kept"])
}
