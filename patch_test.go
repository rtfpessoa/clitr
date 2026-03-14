package main

import (
	"encoding/json"
	"testing"

	"github.com/rtfpessoa/clitr/internal/patch"
	"github.com/stretchr/testify/assert"
)

func TestFormatSuggestions_SingleField(t *testing.T) {
	fields := []patch.UnknownField{
		{
			StructName:  "TimelineEvent",
			StructFile:  "internal/types/event.go",
			JSONKey:     "newField",
			RawValue:    json.RawMessage(`"hello"`),
			GoType:      "string",
			GoFieldName: "NewField",
		},
	}

	output := formatSuggestions(fields)

	assert.Contains(t, output, "Found 1 unknown field(s) across 1 struct(s):")
	assert.Contains(t, output, "Struct: TimelineEvent (internal/types/event.go)")
	assert.Contains(t, output, `+ NewField string`)
	assert.Contains(t, output, `json:"newField,omitempty"`)
}

func TestFormatSuggestions_MultipleStructs(t *testing.T) {
	fields := []patch.UnknownField{
		{
			StructName:  "TimelineEvent",
			StructFile:  "internal/types/event.go",
			JSONKey:     "fieldA",
			GoType:      "string",
			GoFieldName: "FieldA",
		},
		{
			StructName:  "Section",
			StructFile:  "internal/types/raw.go",
			JSONKey:     "fieldB",
			GoType:      "bool",
			GoFieldName: "FieldB",
		},
		{
			StructName:  "TimelineEvent",
			StructFile:  "internal/types/event.go",
			JSONKey:     "fieldC",
			GoType:      "float64",
			GoFieldName: "FieldC",
		},
	}

	output := formatSuggestions(fields)

	assert.Contains(t, output, "Found 3 unknown field(s) across 2 struct(s):")
	assert.Contains(t, output, "Struct: Section (internal/types/raw.go)")
	assert.Contains(t, output, "Struct: TimelineEvent (internal/types/event.go)")
}
