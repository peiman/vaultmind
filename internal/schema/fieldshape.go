package schema

import (
	"fmt"
	"time"
)

// fieldShapes is what the readers accept for each recognized field. The
// indexer reads text fields as strings and list fields as a string or a list
// of strings, dropping anything else without a word — so a value of the wrong
// shape is not rejected on read, it is lost. Checking at write time is the
// only point where the writer can still be told.
var fieldShapes = map[string]string{
	"title": shapeText, "status": shapeText, "parent_id": shapeText,
	"aliases": shapeTextList, "tags": shapeTextList, "related_ids": shapeTextList,
	"source_ids": shapeTextList, "paths": shapeTextList,
	"created": shapeDate, "updated": shapeDate,
}

const (
	shapeText     = "text"
	shapeTextList = "text or a list of text"
	shapeDate     = "a date"
)

// ValueShapeError says why value does not fit field, or returns "" when it
// fits. A field the schema does not shape (an extra, or a type-specific
// field) takes any value. An alias is checked as its canonical field.
func (r *Registry) ValueShapeError(field string, value interface{}) string {
	shape, ok := fieldShapes[r.canonicalName(field)]
	if !ok || fitsShape(shape, value) {
		return ""
	}
	return fmt.Sprintf("field %q takes %s; got %s", field, shape, describeValue(value))
}

func (r *Registry) canonicalName(field string) string {
	for canonical := range fieldShapes {
		if r.IsAlias(canonical, field) {
			return canonical
		}
	}
	return field
}

func fitsShape(shape string, value interface{}) bool {
	switch v := value.(type) {
	case string:
		return true
	case time.Time:
		return shape == shapeDate
	case []interface{}:
		if shape != shapeTextList {
			return false
		}
		for _, item := range v {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	case []string:
		return shape == shapeTextList
	}
	return false
}

// describeValue names a value the way the person who wrote it would: YAML and
// JSON numbers arrive as int or float64, and "float64" is not what they typed.
func describeValue(value interface{}) string {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			if _, ok := item.(string); !ok {
				return "a list holding " + describeValue(item)
			}
		}
		return "a list"
	case map[string]interface{}:
		return "a map"
	case nil:
		return "nothing"
	case bool:
		return "true/false"
	case int, int64, float64:
		return "a number"
	case time.Time:
		return "a date"
	}
	return fmt.Sprintf("%T", value)
}
