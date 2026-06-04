package hooks

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/hookscripts"
)

// RemoveStanza strips every hook entry that references a VaultMind-installed
// script (any name in hookscripts.Names()) from a .claude/settings.json (or
// settings.local.json) payload — the inverse of MergeStanza. An event array
// left empty by the removal is dropped, and the hooks object itself is dropped
// if it becomes empty, so uninstall leaves no orphaned scaffolding. All other
// content and top-level key order are preserved.
//
// Only entries that reference our canonical scripts are touched: a project's
// own hooks, and consumer-owned wrappers (e.g. auto-rag-config.sh, which is not
// one of our script names), are left intact. Returns the sorted, unique set of
// our script basenames that were removed; an empty set means nothing matched.
// Returns the input bytes verbatim when there was nothing to remove (idempotent
// re-run). Malformed JSON returns an error and nil output.
func RemoveStanza(existing []byte) ([]byte, []string, error) {
	top, err := parseOrderedObject(existing)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing settings: %w", err)
	}
	hooksRaw, ok := top.get("hooks")
	if !ok {
		return existing, nil, nil // no hooks → nothing to remove
	}
	hooksObj, err := parseOrderedObjectFromRaw(hooksRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing existing hooks: %w", err)
	}

	names := hookscripts.Names()
	removed := map[string]struct{}{}
	changed := false

	// Snapshot event keys: we mutate hooksObj (remove/set) while iterating.
	for _, event := range append([]string(nil), hooksObj.keys...) {
		raw, _ := hooksObj.get(event)
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, nil, fmt.Errorf("parsing hooks.%s array: %w", event, err)
		}
		kept := make([]json.RawMessage, 0, len(arr))
		for _, el := range arr {
			matches := groupReferencedScriptNames(el, names)
			if len(matches) > 0 {
				for _, m := range matches {
					removed[m] = struct{}{}
				}
				changed = true
				continue // drop this entry
			}
			kept = append(kept, el)
		}
		if len(kept) == len(arr) {
			continue // untouched event
		}
		if len(kept) == 0 {
			hooksObj.remove(event)
			continue
		}
		newArr, err := json.Marshal(kept)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling hooks.%s array: %w", event, err)
		}
		hooksObj.set(event, newArr)
	}

	if !changed {
		return existing, nil, nil
	}

	if hooksObj.len() == 0 {
		top.remove("hooks")
	} else {
		newHooks, err := hooksObj.marshal()
		if err != nil {
			return nil, nil, fmt.Errorf("rendering hooks object: %w", err)
		}
		top.set("hooks", newHooks)
	}
	out, err := top.marshalIndent()
	if err != nil {
		return nil, nil, fmt.Errorf("rendering settings: %w", err)
	}
	return out, sortedKeys(removed), nil
}

// groupReferencedScriptNames returns the VaultMind script basenames (from
// names) that a hook group's commands mention. A group whose shape doesn't
// decode matches nothing, so foreign content is never removed.
func groupReferencedScriptNames(el json.RawMessage, names []string) []string {
	var g hookGroup
	if err := json.Unmarshal(el, &g); err != nil {
		return nil
	}
	var matched []string
	for _, n := range names {
		for _, h := range g.Hooks {
			if commandReferencesScript(h.Command, n) {
				matched = append(matched, n)
				break
			}
		}
	}
	return matched
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
