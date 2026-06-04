package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MergeStanza additively merges VaultMind's five canonical hook entries into
// an existing .claude/settings.json (or settings.local.json) byte payload.
//
// The merge is strictly additive and never clobbers: for each of the five
// events it appends VaultMind's entry only if no entry already in that event's
// array references the corresponding script (dedup by script basename), so a
// project's own hooks — and a re-run of this command — are preserved untouched.
// Top-level keys and their order are kept; only the hooks subtree is rewritten,
// so the diff a user reviews before committing is minimal.
//
// existing may be empty/whitespace (a fresh file) → the full stanza is written.
// vaultPath, when non-empty, is baked into each command via VAULTMIND_VAULT
// (identical to SettingsStanza). Returns changed=false, and the input bytes
// verbatim, when nothing was appended (idempotent re-run). Malformed existing
// JSON returns an error and nil output, so a caller never writes a corrupted
// file over a user's settings.
func MergeStanza(existing []byte, vaultPath string) ([]byte, bool, error) {
	top, err := parseOrderedObject(existing)
	if err != nil {
		return nil, false, fmt.Errorf("parsing settings: %w", err)
	}

	hooksObj := newOrderedObject()
	if raw, ok := top.get("hooks"); ok {
		hooksObj, err = parseOrderedObjectFromRaw(raw)
		if err != nil {
			return nil, false, fmt.Errorf("parsing existing hooks: %w", err)
		}
	}

	changed := false
	for _, ch := range canonicalHooks(vaultPath) {
		var arr []json.RawMessage
		if raw, ok := hooksObj.get(ch.Event); ok {
			if err := json.Unmarshal(raw, &arr); err != nil {
				return nil, false, fmt.Errorf("parsing hooks.%s array: %w", ch.Event, err)
			}
		}
		if anyGroupReferencesScript(arr, ch.Script) {
			continue // already wired (by us on a prior run, or by hand) — never duplicate
		}
		groupRaw, err := json.Marshal(ch.Group)
		if err != nil {
			return nil, false, fmt.Errorf("marshaling %s entry: %w", ch.Event, err)
		}
		arr = append(arr, groupRaw)
		newArr, err := json.Marshal(arr)
		if err != nil {
			return nil, false, fmt.Errorf("marshaling hooks.%s array: %w", ch.Event, err)
		}
		hooksObj.set(ch.Event, newArr)
		changed = true
	}

	if !changed {
		// Nothing to add — return the caller's bytes verbatim. We never
		// reformat a file we aren't otherwise changing (minimal mutation).
		return existing, false, nil
	}

	hooksRaw, err := hooksObj.marshal()
	if err != nil {
		return nil, false, fmt.Errorf("rendering hooks object: %w", err)
	}
	top.set("hooks", hooksRaw)
	out, err := top.marshalIndent()
	if err != nil {
		return nil, false, fmt.Errorf("rendering settings: %w", err)
	}
	return out, true, nil
}

// anyGroupReferencesScript reports whether any hook group in arr has a command
// invoking basename — the dedup primitive for both merge (skip-if-present)
// and uninstall (remove-if-present). A group whose shape doesn't decode is
// treated as "not ours" so foreign content is never matched away.
func anyGroupReferencesScript(arr []json.RawMessage, basename string) bool {
	for _, el := range arr {
		var g hookGroup
		if err := json.Unmarshal(el, &g); err != nil {
			continue
		}
		for _, h := range g.Hooks {
			if commandReferencesScript(h.Command, basename) {
				return true
			}
		}
	}
	return false
}

// commandReferencesScript reports whether a hook command invokes the VaultMind
// script basename. It anchors on a leading path separator and a trailing token
// boundary so a user's own script with an overlapping name is never matched —
// neither a prefix collision (my-vault-recall.sh) nor a suffix one
// (vault-recall.sh.bak). Every command VaultMind emits invokes the script as
// "…/.claude/scripts/<basename>", so the name is always preceded by '/'.
// Matching foreign content here would be a data-safety bug: merge would skip
// installing our hook, and uninstall would delete the user's hook.
func commandReferencesScript(command, basename string) bool {
	needle := "/" + basename
	from := 0
	for {
		i := strings.Index(command[from:], needle)
		if i < 0 {
			return false
		}
		end := from + i + len(needle)
		if end == len(command) || isCommandBoundary(command[end]) {
			return true
		}
		from += i + 1
	}
}

// isCommandBoundary reports whether b terminates a script-path token in a shell
// command, so a trailing ".bak"/"x" suffix on an overlapping name doesn't match.
func isCommandBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '"', '\'', '`', ';', '&', '|', ')', '<', '>':
		return true
	default:
		return false
	}
}

// orderedObject is a JSON object that preserves key order across a
// parse → edit → emit round-trip. Go's map-backed json (un)marshaling sorts
// keys, which would scramble a user's settings.json on every merge; this keeps
// the diff limited to the hooks subtree we actually touch.
type orderedObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func newOrderedObject() *orderedObject {
	return &orderedObject{values: map[string]json.RawMessage{}}
}

func (o *orderedObject) get(k string) (json.RawMessage, bool) {
	v, ok := o.values[k]
	return v, ok
}

// set inserts or updates k, appending to the key order only on first insert so
// updates keep a key in its original position.
func (o *orderedObject) set(k string, v json.RawMessage) {
	if _, ok := o.values[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.values[k] = v
}

// remove deletes k from both the value map and the key order. No-op if absent.
func (o *orderedObject) remove(k string) {
	if _, ok := o.values[k]; !ok {
		return
	}
	delete(o.values, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *orderedObject) len() int { return len(o.keys) }

// parseOrderedObject parses a JSON object preserving key order. Empty or
// all-whitespace input yields an empty object so a fresh settings file merges
// cleanly; non-object JSON (array, scalar) is an error.
func parseOrderedObject(data []byte) (*orderedObject, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return newOrderedObject(), nil
	}
	return parseOrderedObjectFromRaw(data)
}

func parseOrderedObjectFromRaw(data []byte) (*orderedObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("reading JSON: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}
	obj := newOrderedObject()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading object key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %v", keyTok)
		}
		// Reject duplicate keys: keeping "last value wins" would silently drop
		// the earlier value on re-emit — a data loss we must not mask.
		if _, dup := obj.values[key]; dup {
			return nil, fmt.Errorf("duplicate key %q in JSON object", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("reading value for %q: %w", key, err)
		}
		obj.set(key, raw)
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, fmt.Errorf("reading object close: %w", err)
	}
	// Reject trailing content after the object. We re-emit only the parsed
	// object, so silently accepting trailing bytes would drop them from the
	// rewritten file. (A bounded json.RawMessage value has none; a whole file
	// with garbage after the top-level object does.)
	if _, err := dec.Token(); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("unexpected content after JSON object: %w", err)
		}
		return nil, fmt.Errorf("unexpected content after JSON object")
	}
	return obj, nil
}

// marshal renders the object as compact JSON in key order.
func (o *orderedObject) marshal() (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		var vc bytes.Buffer
		if err := json.Compact(&vc, o.values[k]); err != nil {
			return nil, fmt.Errorf("compacting value for %q: %w", k, err)
		}
		buf.Write(vc.Bytes())
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes()), nil
}

// marshalIndent renders pretty-printed JSON (2-space) in key order with a
// trailing newline, matching the .claude/settings.json convention.
func (o *orderedObject) marshalIndent() ([]byte, error) {
	compact, err := o.marshal()
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact, "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}
