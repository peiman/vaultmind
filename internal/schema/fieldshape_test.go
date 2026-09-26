package schema_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
)

func TestValueShapeError_AnAliasIsCheckedAsItsCanonicalField(t *testing.T) {
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{"concept": {Required: []string{"title"}}},
		map[string][]string{"updated": {"last_updated"}})

	assert.Empty(t, reg.ValueShapeError("last_updated", "2026-09-26"))
	assert.Equal(t, `field "last_updated" takes a date; got a number`, reg.ValueShapeError("last_updated", 5))
}

func TestValueShapeError_NamesWhatWasWrong(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{"concept": {Required: []string{"title"}}})

	assert.Equal(t, `field "title" takes text; got a list`, reg.ValueShapeError("title", []interface{}{"a"}))
	assert.Equal(t, `field "tags" takes text or a list of text; got a list holding a number`,
		reg.ValueShapeError("tags", []interface{}{"a", 3}))
	assert.Equal(t, `field "tags" takes text or a list of text; got a map`,
		reg.ValueShapeError("tags", map[string]interface{}{"a": 1}))
	assert.Equal(t, `field "title" takes text; got nothing`, reg.ValueShapeError("title", nil))
	assert.Equal(t, `field "title" takes text; got a date`, reg.ValueShapeError("title", time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, `field "status" takes text; got true/false`, reg.ValueShapeError("status", true))
	assert.Equal(t, `field "tags" takes text or a list of text; got schema_test.odd`, reg.ValueShapeError("tags", odd{}))
	assert.Empty(t, reg.ValueShapeError("tags", []string{"a"}))
	assert.Empty(t, reg.ValueShapeError("tags", []interface{}{"a", "b"}))
	assert.Empty(t, reg.ValueShapeError("owner_id", 42), "a field the schema does not shape takes any value")
}

type odd struct{}
