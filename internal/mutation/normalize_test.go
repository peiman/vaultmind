package mutation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortKeys_CanonicalOrder(t *testing.T) {
	raw := []byte("---\ntags:\n  - test\nstatus: active\ntitle: My Note\nid: test-note\ntype: project\ncreated: 2026-01-01\nupdated: 2026-01-02\ncustom_field: value\nalpha_field: data\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	SortKeys(node.Content[0])
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)

	idIdx := bytes.Index(out, []byte("id:"))
	typeIdx := bytes.Index(out, []byte("type:"))
	statusIdx := bytes.Index(out, []byte("status:"))
	titleIdx := bytes.Index(out, []byte("title:"))
	tagsIdx := bytes.Index(out, []byte("tags:"))
	createdIdx := bytes.Index(out, []byte("created:"))
	updatedIdx := bytes.Index(out, []byte("updated:"))
	alphaIdx := bytes.Index(out, []byte("alpha_field:"))
	customIdx := bytes.Index(out, []byte("custom_field:"))

	s := string(out)
	assert.Less(t, idIdx, typeIdx, "id < type in: %s", s)
	assert.Less(t, typeIdx, statusIdx, "type < status in: %s", s)
	assert.Less(t, statusIdx, titleIdx, "status < title in: %s", s)
	assert.Less(t, titleIdx, tagsIdx, "title < tags in: %s", s)
	assert.Less(t, tagsIdx, createdIdx, "tags < created in: %s", s)
	assert.Less(t, createdIdx, updatedIdx, "created < updated in: %s", s)
	assert.Less(t, updatedIdx, alphaIdx, "updated < alpha in: %s", s)
	assert.Less(t, alphaIdx, customIdx, "alpha < custom in: %s", s)
}

func TestSortKeys_MissingCanonicalKeys(t *testing.T) {
	raw := []byte("---\nzebra: z\nid: test\ntype: concept\nalpha: a\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	SortKeys(node.Content[0])
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)

	idIdx := bytes.Index(out, []byte("id:"))
	typeIdx := bytes.Index(out, []byte("type:"))
	alphaIdx := bytes.Index(out, []byte("alpha:"))
	zebraIdx := bytes.Index(out, []byte("zebra:"))

	assert.Less(t, idIdx, typeIdx)
	assert.Less(t, typeIdx, alphaIdx)
	assert.Less(t, alphaIdx, zebraIdx)
}

func TestScalarToList_StringToList(t *testing.T) {
	raw := []byte("---\nid: test\naliases: My Alias\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	changed := ScalarToList(node.Content[0], "aliases")
	assert.True(t, changed)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "aliases:\n")
	assert.Contains(t, string(out), "- My Alias")
}

func TestScalarToList_AlreadyList(t *testing.T) {
	raw := []byte("---\nid: test\naliases:\n  - One\n  - Two\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	changed := ScalarToList(node.Content[0], "aliases")
	assert.False(t, changed)
}

func TestScalarToList_KeyNotPresent(t *testing.T) {
	raw := []byte("---\nid: test\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	changed := ScalarToList(node.Content[0], "aliases")
	assert.False(t, changed)
}

func TestScalarToList_Tags(t *testing.T) {
	raw := []byte("---\nid: test\ntags: billing\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	changed := ScalarToList(node.Content[0], "tags")
	assert.True(t, changed)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "- billing")
}

func TestNormalizeDates_StripMidnightTime(t *testing.T) {
	raw := []byte("---\nid: test\ncreated: 2026-04-04T00:00:00\nupdated: 2026-04-04T00:00:00Z\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	NormalizeDates(node.Content[0], false)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "2026-04-04")
	assert.NotContains(t, s, "T00:00:00")
}

func TestNormalizeDates_PreserveNonMidnight(t *testing.T) {
	raw := []byte("---\nid: test\ncreated: 2026-04-04T14:30:00Z\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	NormalizeDates(node.Content[0], false)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "14:30")
}

func TestNormalizeDates_StripTimeForced(t *testing.T) {
	raw := []byte("---\nid: test\ncreated: 2026-04-04T14:30:00Z\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	NormalizeDates(node.Content[0], true)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "2026-04-04")
	assert.NotContains(t, s, "14:30")
}

func TestNormalizeDates_AlreadyDateOnly(t *testing.T) {
	raw := []byte("---\nid: test\ncreated: 2026-04-04\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	NormalizeDates(node.Content[0], false)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "2026-04-04")
}

func TestSnakeCaseKeys_ConvertsCamelCase(t *testing.T) {
	raw := []byte("---\nid: test\nownerId: person-1\nrelatedIds:\n  - proj-1\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	renames := SnakeCaseKeys(node.Content[0])
	assert.Len(t, renames, 2)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "owner_id:")
	assert.Contains(t, s, "related_ids:")
	assert.NotContains(t, s, "ownerId")
	assert.NotContains(t, s, "relatedIds")
}

func TestSnakeCaseKeys_AlreadySnakeCase(t *testing.T) {
	raw := []byte("---\nid: test\nowner_id: person-1\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	renames := SnakeCaseKeys(node.Content[0])
	assert.Empty(t, renames)
}

func TestSnakeCaseKeys_PreservesValues(t *testing.T) {
	raw := []byte("---\nid: test\nownerId: person-alice\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	SnakeCaseKeys(node.Content[0])
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "owner_id: person-alice")
}
