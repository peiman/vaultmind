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
func eventWiring(projectDir string) []EventStatus {
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
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}

	wired := func(event, script string) bool {
		for _, group := range parsed.Hooks[event] {
			for _, h := range group.Hooks {
				if strings.Contains(h.Command, script) {
					return true
				}
			}
		}
		return false
	}

	// vaultPath is only used to build command strings for install; wiring
	// detection does not depend on it, so an empty value is correct here.
	canonical := canonicalHooks("")
	out := make([]EventStatus, 0, len(canonical))
	for _, c := range canonical {
		state := EventUnwired
		if wired(c.Event, c.Script) {
			state = EventWired
		}
		out = append(out, EventStatus{Event: c.Event, Script: c.Script, State: state})
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
