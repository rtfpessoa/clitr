package patch

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ApplyPatches modifies Go source files to add missing struct fields.
// It handles three cases:
// 1. Named struct types: adds field to the struct definition
// 2. Alias-pattern UnmarshalJSON: only the struct definition needs updating (alias delegates)
// 3. Inner raw struct UnmarshalJSON: updates struct def, inner raw struct, and field assignment
// If sourceDir is non-empty, relative StructFile paths are resolved against it.
func ApplyPatches(fields []UnknownField, sourceDir ...string) error {
	if len(fields) == 0 {
		return nil
	}

	var baseDir string
	if len(sourceDir) > 0 && sourceDir[0] != "" {
		baseDir = sourceDir[0]
	}

	// Group fields by target file, resolving paths
	grouped := make(map[string][]UnknownField)
	for _, f := range fields {
		path := f.StructFile
		if baseDir != "" && !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		grouped[path] = append(grouped[path], f)
	}

	for filePath, fileFields := range grouped {
		if err := applyToFile(filePath, fileFields); err != nil {
			return fmt.Errorf("failed to patch %s: %w", filePath, err)
		}
	}

	return nil
}

// applyToFile parses a Go source file and adds missing fields to the appropriate structs.
// Uses a two-pass approach:
// Pass 1 (AST): Add fields to named structs and inner raw structs
// Pass 2 (Text): Add field assignment lines to UnmarshalJSON methods
func applyToFile(filePath string, fields []UnknownField) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	plan := newPatchPlan(fields)
	ast.Inspect(f, plan.inspect)
	if !plan.modified {
		return nil
	}

	// Ensure json import exists if we added json.RawMessage fields
	for _, field := range fields {
		if field.GoType == "json.RawMessage" {
			ensureImport(f, "encoding/json")
			break
		}
	}

	// Write the AST-modified file
	if err := writeFile(fset, f, filePath); err != nil {
		return err
	}

	// Pass 2: Text-based assignment line insertion
	if len(plan.assignments) > 0 {
		return insertAssignmentLines(filePath, plan.assignments)
	}

	return nil
}

type patchPlan struct {
	fieldsByStruct map[string][]UnknownField
	assignments    []assignmentInfo
	modified       bool
}

func newPatchPlan(fields []UnknownField) *patchPlan {
	plan := &patchPlan{fieldsByStruct: make(map[string][]UnknownField)}
	for _, field := range fields {
		plan.fieldsByStruct[field.StructName] = append(plan.fieldsByStruct[field.StructName], field)
	}
	return plan
}

func (p *patchPlan) inspect(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		p.patchType(node)
	case *ast.FuncDecl:
		p.patchUnmarshalMethod(node)
	}
	return true
}

func (p *patchPlan) patchType(node *ast.TypeSpec) {
	fields := p.fieldsByStruct[node.Name.Name]
	st, ok := node.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range fields {
		if addFieldToStruct(st, field) {
			p.modified = true
		}
	}
}

func (p *patchPlan) patchUnmarshalMethod(node *ast.FuncDecl) {
	if node.Name.Name != "UnmarshalJSON" || node.Recv == nil || len(node.Recv.List) == 0 {
		return
	}
	receiver := node.Recv.List[0]
	fields := p.fieldsByStruct[getReceiverTypeName(receiver.Type)]
	if len(fields) == 0 || !patchInnerRawStruct(node, fields) {
		return
	}
	p.modified = true
	p.assignments = append(p.assignments, assignmentInfo{
		recvVar: getReceiverVarName(receiver),
		fields:  fields,
	})
}

// addFieldToStruct adds a field to a struct if it doesn't already exist (by json tag).
func addFieldToStruct(st *ast.StructType, field UnknownField) bool {
	if hasJSONTag(st, field.JSONKey) {
		return false
	}

	newField := &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(field.GoFieldName)},
		Type:  parseTypeExpr(field.GoType),
		Tag: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("`json:\"%s,omitempty\"`", field.JSONKey),
		},
	}

	st.Fields.List = append(st.Fields.List, newField)
	return true
}

// hasJSONTag checks if a struct already has a field with the given JSON tag name.
func hasJSONTag(st *ast.StructType, jsonKey string) bool {
	for _, field := range st.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag := field.Tag.Value
		if strings.Contains(tag, `json:"`+jsonKey+`"`) || strings.Contains(tag, `json:"`+jsonKey+`,`) {
			return true
		}
	}
	return false
}

// parseTypeExpr converts a type string to an AST expression.
func parseTypeExpr(goType string) ast.Expr {
	if strings.Contains(goType, ".") {
		parts := strings.SplitN(goType, ".", 2)
		return &ast.SelectorExpr{
			X:   ast.NewIdent(parts[0]),
			Sel: ast.NewIdent(parts[1]),
		}
	}
	return ast.NewIdent(goType)
}

// patchInnerRawStruct adds fields to the inner "var raw struct { ... }" in an UnmarshalJSON method.
func patchInnerRawStruct(funcDecl *ast.FuncDecl, fields []UnknownField) bool {
	if funcDecl.Body == nil {
		return false
	}
	structs := findInnerRawStructs(funcDecl.Body)
	if len(structs) == 0 {
		return false
	}
	modified := false
	for _, st := range structs {
		for _, field := range fields {
			if addFieldToStruct(st, field) {
				modified = true
			}
		}
	}
	return modified
}

func findInnerRawStructs(body *ast.BlockStmt) []*ast.StructType {
	var structs []*ast.StructType
	ast.Inspect(body, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, name := range value.Names {
			if name.Name == "raw" {
				if st, ok := value.Type.(*ast.StructType); ok {
					structs = append(structs, st)
				}
				return false
			}
		}
		return true
	})
	return structs
}

// assignmentInfo pairs a receiver variable with the fields that need assignment lines.
type assignmentInfo struct {
	recvVar string
	fields  []UnknownField
}

// insertAssignmentLines inserts "recv.Field = raw.Field" lines using text manipulation.
// This avoids Go AST comment-positioning issues that occur with AST-based insertion.
func insertAssignmentLines(filePath string, infos []assignmentInfo) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Build a map from receiver variable to fields
	recvToFields := make(map[string][]UnknownField)
	for _, info := range infos {
		recvToFields[info.recvVar] = info.fields
	}

	lines := strings.Split(string(content), "\n")
	var result []string

	for i := 0; i < len(lines); i++ {
		result = append(result, lines[i])

		trimmed := strings.TrimSpace(lines[i])
		if !isRawAssignmentLine(trimmed) {
			continue
		}

		// Check if next non-empty line is also an assignment
		if isNextNonEmptyLineRawAssignment(lines, i) {
			continue
		}

		// This is the last assignment in a block. Determine the receiver variable.
		recvVar := extractRecvVarFromLine(trimmed)
		if recvVar == "" {
			continue
		}

		// Only insert fields for the matching receiver variable
		fields, ok := recvToFields[recvVar]
		if !ok {
			continue
		}

		for _, field := range fields {
			assignText := fmt.Sprintf("\t%s.%s = raw.%s", recvVar, field.GoFieldName, field.GoFieldName)
			if !containsLine(lines, assignText) {
				result = append(result, assignText)
			}
		}

		// Mark as done so we don't insert again for a different block with same receiver
		delete(recvToFields, recvVar)
	}

	output := strings.Join(result, "\n")
	formatted, err := format.Source([]byte(output))
	if err != nil {
		return os.WriteFile(filePath, []byte(output), 0644)
	}

	return os.WriteFile(filePath, formatted, 0644)
}

// isRawAssignmentLine checks if a line matches "x.Y = raw.Y".
func isRawAssignmentLine(line string) bool {
	if !strings.Contains(line, " = raw.") {
		return false
	}
	parts := strings.SplitN(line, " = ", 2)
	if len(parts) != 2 {
		return false
	}
	if !strings.Contains(parts[0], ".") {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(parts[1]), "raw.")
}

// isNextNonEmptyLineRawAssignment checks if the next non-empty line is also a raw assignment.
func isNextNonEmptyLineRawAssignment(lines []string, i int) bool {
	for j := i + 1; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" {
			continue
		}
		return isRawAssignmentLine(trimmed)
	}
	return false
}

// extractRecvVarFromLine extracts "s" from "s.Field = raw.Field".
func extractRecvVarFromLine(line string) string {
	dotIdx := strings.Index(line, ".")
	if dotIdx <= 0 {
		return ""
	}
	return strings.TrimSpace(line[:dotIdx])
}

// containsLine checks if a specific line text exists in the lines array.
func containsLine(lines []string, target string) bool {
	targetTrimmed := strings.TrimSpace(target)
	for _, line := range lines {
		if strings.TrimSpace(line) == targetTrimmed {
			return true
		}
	}
	return false
}

// getReceiverTypeName extracts the type name from a receiver expression.
func getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// getReceiverVarName extracts the variable name from a receiver field.
func getReceiverVarName(field *ast.Field) string {
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	return ""
}

// ensureImport adds an import if it doesn't already exist.
func ensureImport(f *ast.File, importPath string) {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"`+importPath+`"` {
			return
		}
	}

	newImport := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%s"`, importPath),
		},
	}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		gd.Specs = append(gd.Specs, newImport)
		return
	}

	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{newImport},
	}
	f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
}

// writeFile formats and writes an AST back to a file.
func writeFile(fset *token.FileSet, f *ast.File, filePath string) error {
	var output bytes.Buffer
	if err := format.Node(&output, fset, f); err != nil {
		return fmt.Errorf("failed to format AST: %w", err)
	}
	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format source: %w", err)
	}
	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}
