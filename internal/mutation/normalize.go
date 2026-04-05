package mutation

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var canonicalKeyOrder = []string{
	"id", "type", "status", "title", "aliases", "tags", "created", "updated",
}

// SortKeys sorts the keys of a YAML mapping node: canonical keys first (in
// canonical order), then remaining keys alphabetically.
func SortKeys(mapping *yaml.Node) {
	if mapping.Kind != yaml.MappingNode {
		return
	}
	type kvPair struct {
		key *yaml.Node
		val *yaml.Node
	}
	pairs := make([]kvPair, 0, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		pairs = append(pairs, kvPair{mapping.Content[i], mapping.Content[i+1]})
	}

	priority := make(map[string]int)
	for i, k := range canonicalKeyOrder {
		priority[k] = i
	}
	maxP := len(canonicalKeyOrder)

	sort.SliceStable(pairs, func(i, j int) bool {
		pi, oki := priority[pairs[i].key.Value]
		pj, okj := priority[pairs[j].key.Value]
		if !oki {
			pi = maxP
		}
		if !okj {
			pj = maxP
		}
		if pi != pj {
			return pi < pj
		}
		return pairs[i].key.Value < pairs[j].key.Value
	})

	mapping.Content = make([]*yaml.Node, 0, len(pairs)*2)
	for _, p := range pairs {
		mapping.Content = append(mapping.Content, p.key, p.val)
	}
}

// ScalarToList converts a scalar value at the given key into a single-element
// sequence node. Returns true if a conversion was performed, false otherwise
// (key absent, value already a sequence, etc.).
func ScalarToList(mapping *yaml.Node, key string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			if valNode.Kind == yaml.ScalarNode {
				itemNode := &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: valNode.Value,
					Tag:   valNode.Tag,
					Style: valNode.Style,
				}
				mapping.Content[i+1] = &yaml.Node{
					Kind:    yaml.SequenceNode,
					Tag:     "!!seq",
					Content: []*yaml.Node{itemNode},
				}
				return true
			}
			return false
		}
	}
	return false
}

var dateFields = map[string]bool{"created": true, "updated": true, "due": true}

var dateTimeFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02",
}

// NormalizeDates strips the time component from date-field scalar values.
// When stripTime is false, only midnight timestamps are stripped. When
// stripTime is true, all time components are removed regardless.
func NormalizeDates(mapping *yaml.Node, stripTime bool) {
	if mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		key := mapping.Content[i].Value
		if !dateFields[key] {
			continue
		}
		valNode := mapping.Content[i+1]
		if valNode.Kind != yaml.ScalarNode {
			continue
		}
		for _, format := range dateTimeFormats {
			t, err := time.Parse(format, valNode.Value)
			if err != nil {
				continue
			}
			isMidnight := t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0
			if stripTime || isMidnight {
				valNode.Value = t.Format("2006-01-02")
				valNode.Tag = "!!str"
				valNode.Style = yaml.DoubleQuotedStyle
			}
			break
		}
	}
}

var camelCaseRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// KeyRename records a key rename performed by SnakeCaseKeys.
type KeyRename struct {
	OldKey string
	NewKey string
}

// SnakeCaseKeys converts camelCase key names in a mapping node to snake_case
// in-place and returns the list of renames performed.
func SnakeCaseKeys(mapping *yaml.Node) []KeyRename {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	var renames []KeyRename
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		keyNode := mapping.Content[i]
		oldKey := keyNode.Value
		newKey := toSnakeCase(oldKey)
		if newKey != oldKey {
			keyNode.Value = newKey
			renames = append(renames, KeyRename{OldKey: oldKey, NewKey: newKey})
		}
	}
	return renames
}

func toSnakeCase(s string) string {
	snake := camelCaseRe.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}
