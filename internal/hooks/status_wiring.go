package hooks

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// EventState says whether a canonical event is actually wired in the project's
// settings.json.
type EventState string

const (
	// EventWired — settings.json runs this canonical script on this event.
	EventWired EventState = "wired"
	// EventUnwired — the binary wires this event and the project does not.
	// Reported separately because an absence that renders as nothing looks
	// exactly like health: the whole reason this command exists, applied to the
	// event map rather than to the script files.
	EventUnwired EventState = "unwired"
	// EventStaleMatcher — the script is wired, but under a matcher an earlier
	// release installed, so it misses tools it now needs to see (the reach hook
	// on "Bash" cannot see Edit or Write). `hooks install --merge` upgrades it.
	EventStaleMatcher EventState = "stale_matcher"
)

// EventStatus is one canonical event→script pair and whether it is live.
type EventStatus struct {
	Event  string     `json:"event"`
	Script string     `json:"script"`
	State  EventState `json:"state"`
}

// eventWiring compares the canonical event map against a project's settings.
//
// Content comparison cannot see this class at all. A project can hold every
// canonical script, byte-identical, and still never run one of them — which is
// exactly what happened to an adopter whose SessionEnd was absent while
// capture-episode.sh sat on disk having already produced 13 episodes. Every
// content check passed; the write half was off.
func eventWiringForProfile(projectDir string, p Profile) []EventStatus {
	raw, err := readSettingsFile(filepath.Join(projectDir, ".claude", "settings.json"))
	if err != nil || len(raw) == 0 {
		raw = nil
	}

	// Match on the script filename appearing in the command for that event.
	// Deliberately not an exact-string match on the whole command: projects
	// legitimately template the path ($CLAUDE_PROJECT_DIR, absolute, relative),
	// and treating a path spelling difference as "unwired" would produce false
	// alarms — which is how a check gets ignored, then deleted.
	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}

	// A script wired under a matcher an earlier release installed is not
	// healthy: it runs, but misses tools it now needs to see.
	stateOf := func(event, script string) EventState {
		for _, group := range parsed.Hooks[event] {
			for _, h := range group.Hooks {
				if strings.Contains(h.Command, script) {
					if isLegacyMatcher(script, group.Matcher) {
						return EventStaleMatcher
					}
					return EventWired
				}
			}
		}
		return EventUnwired
	}

	// vaultPath is only used to build command strings for install; wiring
	// detection does not depend on it, so an empty value is correct here.
	// Only the events this profile actually runs. A knowledge vault has no
	// persona to load and no episode to capture; reporting those as "unwired"
	// is reporting a choice as a defect.
	expected := EventScriptsForProfile(p)
	out := make([]EventStatus, 0, len(expected))
	for _, c := range expected {
		out = append(out, EventStatus{Event: c.Event, Script: c.Script, State: stateOf(c.Event, c.Script)})
	}
	return out
}

// EventCounts returns how many canonical events are wired and unwired.
func (r StatusReport) EventCounts() (wired, unwired int) {
	for _, e := range r.Events {
		if e.State == EventWired {
			wired++
		} else {
			unwired++
		}
	}
	return wired, unwired
}

// EventScript is one canonical event→script pair, exported so tests and callers
// can build or check a project's wiring without duplicating the map.
type EventScript struct {
	Event  string
	Script string
}

// CanonicalEventScripts lists the event→script pairs this binary wires.
func CanonicalEventScripts() []EventScript {
	canonical := canonicalHooks("")
	out := make([]EventScript, 0, len(canonical))
	for _, c := range canonical {
		out = append(out, EventScript{Event: c.Event, Script: c.Script})
	}
	return out
}
