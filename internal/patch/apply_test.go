package patch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPatches_AddFieldToNamedStruct(t *testing.T) {
	src := `package types

type MyStruct struct {
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age,omitempty\"`" + `
}
`
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte(src), 0644))

	fields := []UnknownField{
		{
			StructName:  "MyStruct",
			StructFile:  filePath,
			JSONKey:     "email",
			GoType:      "string",
			GoFieldName: "Email",
		},
	}

	err := ApplyPatches(fields)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, "Email")
	assert.Contains(t, resultStr, `json:"email,omitempty"`)
	assert.Contains(t, resultStr, "string")

	// Verify it's valid Go
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, filePath, result, parser.ParseComments)
	require.NoError(t, err, "patched file should be valid Go")
}

func TestApplyPatches_AddFieldToInnerRawStruct(t *testing.T) {
	src := `package types

import "encoding/json"

type Section struct {
	Title string ` + "`json:\"title\"`" + `
	Type  string ` + "`json:\"type,omitempty\"`" + `
}

func (s *Section) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title string ` + "`json:\"title\"`" + `
		Type  string ` + "`json:\"type,omitempty\"`" + `
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Title = raw.Title
	s.Type = raw.Type

	return nil
}
`
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte(src), 0644))

	fields := []UnknownField{
		{
			StructName:  "Section",
			StructFile:  filePath,
			JSONKey:     "description",
			GoType:      "string",
			GoFieldName: "Description",
		},
	}

	err := ApplyPatches(fields)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	resultStr := string(result)

	// Should appear in both the struct definition and the inner raw struct
	assert.Equal(t, 2, strings.Count(resultStr, `json:"description,omitempty"`),
		"field should appear in both outer struct and inner raw struct")

	// Should have assignment line
	assert.Contains(t, resultStr, "s.Description = raw.Description",
		"should add field assignment")

	// Verify it's valid Go
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, filePath, result, parser.ParseComments)
	require.NoError(t, err, "patched file should be valid Go")
}

func TestApplyPatches_Idempotent(t *testing.T) {
	src := `package types

type MyStruct struct {
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email,omitempty\"`" + `
}
`
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte(src), 0644))

	fields := []UnknownField{
		{
			StructName:  "MyStruct",
			StructFile:  filePath,
			JSONKey:     "email",
			GoType:      "string",
			GoFieldName: "Email",
		},
	}

	err := ApplyPatches(fields)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	// Field should appear exactly once (already existed)
	assert.Equal(t, 1, strings.Count(string(result), `json:"email`),
		"existing field should not be duplicated")
}

func TestApplyPatches_MultipleFields(t *testing.T) {
	src := `package types

type Alpha struct {
	ID string ` + "`json:\"id\"`" + `
}

type Beta struct {
	Name string ` + "`json:\"name\"`" + `
}
`
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte(src), 0644))

	fields := []UnknownField{
		{
			StructName:  "Alpha",
			StructFile:  filePath,
			JSONKey:     "newA",
			GoType:      "bool",
			GoFieldName: "NewA",
		},
		{
			StructName:  "Beta",
			StructFile:  filePath,
			JSONKey:     "newB",
			GoType:      "float64",
			GoFieldName: "NewB",
		},
	}

	err := ApplyPatches(fields)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, `NewA`)
	assert.Contains(t, resultStr, `json:"newA,omitempty"`)
	assert.Contains(t, resultStr, `NewB`)
	assert.Contains(t, resultStr, `json:"newB,omitempty"`)

	// Verify it's valid Go
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, filePath, result, parser.ParseComments)
	require.NoError(t, err, "patched file should be valid Go")
}

func TestApplyPatches_JsonRawMessageType(t *testing.T) {
	src := `package types

import "encoding/json"

type MyStruct struct {
	Name string ` + "`json:\"name\"`" + `
}
`
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte(src), 0644))

	fields := []UnknownField{
		{
			StructName:  "MyStruct",
			StructFile:  filePath,
			JSONKey:     "extra",
			GoType:      "json.RawMessage",
			GoFieldName: "Extra",
		},
	}

	err := ApplyPatches(fields)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, "Extra")
	assert.Contains(t, resultStr, "json.RawMessage")

	// Verify it's valid Go
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, result, parser.ParseComments)
	require.NoError(t, err, "patched file should be valid Go")

	// Check that import exists
	hasJSONImport := false
	for _, imp := range f.Imports {
		if imp.Path.Value == `"encoding/json"` {
			hasJSONImport = true
		}
	}
	assert.True(t, hasJSONImport, "should have encoding/json import")
}

// hasFieldWithJSONTag checks if a struct has a field with the given json tag name.
func hasFieldWithJSONTag(st *ast.StructType, jsonKey string) bool {
	for _, field := range st.Fields.List {
		if field.Tag != nil {
			tag := field.Tag.Value
			if strings.Contains(tag, `json:"`+jsonKey+`"`) || strings.Contains(tag, `json:"`+jsonKey+`,`) {
				return true
			}
		}
	}
	return false
}
