#!/bin/bash
# Deliver the arc layer at session start, in a hook of its own (#357).
#
# Claude Code replaces a SessionStart hook's output with a ~2 KB preview past
# about 10,000 characters, and the cap is per hook (#355). One hook cannot carry
# the identity, the current context, the memory state AND every arc: measured on
# a 30-arc vault, the shared hook had room for 9. This hook carries the arcs.
#
# All persona logic (the served/explore/alternate arm, vault resolution, the
# sidecar log) stays in load-persona.sh beside this script, which sees
# VAULTMIND_PERSONA_PART=arcs and delivers only the arc layer. Its presence here
# is also how load-persona.sh knows to leave the arcs out of its own output.
VAULTMIND_PERSONA_PART=arcs exec bash "$(dirname "$0")/load-persona.sh"
