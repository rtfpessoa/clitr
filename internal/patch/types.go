// Package patch provides detection of unknown JSON fields and auto-patching
// of Go struct definitions to match evolving API responses.
package patch

import "encoding/json"

// UnknownField represents a JSON field found in API responses that has no
// corresponding field in the Go struct definition.
type UnknownField struct {
	// StructName is the Go struct that should contain this field.
	StructName string

	// StructFile is the relative path to the Go source file containing the struct.
	StructFile string

	// JSONKey is the field name as it appears in the JSON response.
	JSONKey string

	// RawValue is the raw JSON value, used for type inference.
	RawValue json.RawMessage

	// GoType is the inferred Go type for this field (e.g., "string", "bool", "json.RawMessage").
	GoType string

	// GoFieldName is the PascalCase Go field name derived from the JSON key.
	GoFieldName string
}
