package hooks

import (
	"encoding/json"
	"path/filepath"
)

// EventState says whether a canonical event is actually wired in the project's
// settings files.
type EventState string

const (
	// EventWired — a settings file runs this canonical script on this event.
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
	// EventOutsideProfile — the project runs one of our hooks that its declared
	// profile excludes (a persona loader in a knowledge project). It runs a
	// script the profile chose not to have; `hooks install --profile <p>
	// --merge` removes it.
	EventOutsideProfile EventState = "outside_profile"
)

// EventStatus is one canonical event→script pair and whether it is live.
type EventStatus struct {
	Event  string     `json:"event"`
	Script string     `json:"script"`
	State  EventState `json:"state"`
	// Files names the settings files wiring this pair, in shared/local order.
	Files []string `json:"files,omitempty"`
	// Warnings names duplicate dispatch across project settings files.
	Warnings []string `json:"warnings,omitempty"`
	// Notes names identical handlers that are deduplicated across settings files.
	Notes []string `json:"notes,omitempty"`
}

// LocalOnly reports whether repairs must target the personal project file.
func (e EventStatus) LocalOnly() bool {
	return len(e.Files) == 1 && e.Files[0] == settingsLocalName
}

// eventWiring compares the canonical event map against a project's settings.
//
// Content comparison cannot see this class at all. A project can hold every
// canonical script, byte-identical, and still never run one of them — which is
// exactly what happened to an adopter whose SessionEnd was absent while
// capture-episode.sh sat on disk having already produced 13 episodes. Every
// content check passed; the write half was off.
func eventWiringForProfile(projectDir string, p Profile) []EventStatus {
	return eventWiringForScope(projectDir, p, ScopeProject)
}

func eventWiringForScope(projectDir string, p Profile, scope Scope) []EventStatus {
	// Match on the script filename appearing in the command for that event.
	// Deliberately not an exact-string match on the whole command: projects
	// legitimately template the path ($CLAUDE_PROJECT_DIR, absolute, relative),
	// and treating a path spelling difference as "unwired" would produce false
	// alarms — which is how a check gets ignored, then deleted.
	type wiringFile struct {
		name  string
		hooks map[string][]hookGroup
	}
	var files []wiringFile
	for _, path := range wiringPaths(projectDir, scope, AgentClaude) {
		raw, err := readSettingsFile(path)
		if err != nil || len(raw) == 0 {
			continue
		}
		var parsed struct {
			Hooks map[string][]hookGroup `json:"hooks"`
		}
		// Preserve the internal status check's treatment of malformed settings.
		// The command's scope-overlap check separately surfaces parse/read errors.
		_ = json.Unmarshal(raw, &parsed)
		files = append(files, wiringFile{filepath.Base(path), parsed.Hooks})
	}

	// A script wired under a matcher an earlier release installed is not
	// healthy: it runs, but misses tools it now needs to see. A script in
	// several groups in one file is hand-wired, a matcher per group: the
	// project's choice, and the merge leaves it alone too.
	statusOf := func(event, script string) EventStatus {
		status := EventStatus{Event: event, Script: script, State: EventUnwired}
		stale := false
		var previous []hookGroup
		for _, file := range files {
			groups := groupsForScript(file.hooks[event], script)
			if len(groups) > 0 {
				status.Files = append(status.Files, file.name)
				status.State = EventWired
			}
			if len(groups) == 1 && isLegacyMatcher(script, groups[0].Matcher) {
				stale = true
			}
			addDuplicateNotices(&status, previous, groups)
			previous = append(previous, groups...)
		}
		if stale {
			status.State = EventStaleMatcher
		}
		return status
	}

	// vaultPath is only used to build command strings for install; wiring
	// detection does not depend on it, so an empty value is correct here.
	// Only the events this profile actually runs. A knowledge vault has no
	// persona to load and no episode to capture; reporting those as "unwired"
	// is reporting a choice as a defect.
	expected := EventScriptsForProfile(p)
	out := make([]EventStatus, 0, len(expected))
	inProfile := map[EventScript]bool{}
	for _, c := range expected {
		inProfile[c] = true
		out = append(out, statusOf(c.Event, c.Script))
	}
	// And the other direction: one of our hooks wired that the profile
	// excludes. Without this a knowledge project running a persona loader
	// reported healthy — the profile was only ever read as "what to expect".
	for _, c := range CanonicalEventScripts() {
		if status := statusOf(c.Event, c.Script); !inProfile[c] && status.State != EventUnwired {
			status.State = EventOutsideProfile
			out = append(out, status)
		}
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
