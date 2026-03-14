package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/patch"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type patchConfig struct {
	*rootConfig

	apply bool
}

func NewPatchCmd(rootConfig *rootConfig) *cobra.Command {
	patchConfig := &patchConfig{
		rootConfig: rootConfig,
	}

	patchCmd := &cobra.Command{
		Use:   "patch",
		Short: "Detect and fix unknown fields in API response structs",
		Long: `Scan saved JSON event files and detect fields that are not yet defined
in the Go struct types. Outputs the required struct field additions.

Use --apply to automatically patch the Go source files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPatch(patchConfig)
		},
	}

	patchCmd.Flags().BoolVar(&patchConfig.apply, "apply", false, "Apply patches directly to Go source files")

	return patchCmd
}

func runPatch(config *patchConfig) error {
	log.Info("Scanning for unknown fields", zap.String("dir", config.eventsDir))

	fields, err := patch.ScanDirectory(config.eventsDir)
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	if len(fields) == 0 {
		fmt.Println("No unknown fields detected. All JSON files match current struct definitions.")
		return nil
	}

	fmt.Print(formatSuggestions(fields))

	if config.apply {
		return applyPatches(fields)
	}

	fmt.Println("\nRun with --apply to automatically patch the source files.")
	return nil
}

// formatSuggestions formats unknown fields into human-readable patch suggestions.
func formatSuggestions(fields []patch.UnknownField) string {
	// Group by struct
	grouped := make(map[string][]patch.UnknownField)
	for _, f := range fields {
		key := f.StructName
		grouped[key] = append(grouped[key], f)
	}

	// Sort struct names for stable output
	structNames := make([]string, 0, len(grouped))
	for name := range grouped {
		structNames = append(structNames, name)
	}
	sort.Strings(structNames)

	var b strings.Builder
	totalFields := len(fields)
	totalStructs := len(grouped)
	fmt.Fprintf(&b, "Found %d unknown field(s) across %d struct(s):\n", totalFields, totalStructs)

	for _, structName := range structNames {
		structFields := grouped[structName]
		// Sort fields by JSON key for stable output
		sort.Slice(structFields, func(i, j int) bool {
			return structFields[i].JSONKey < structFields[j].JSONKey
		})

		fmt.Fprintf(&b, "\nStruct: %s (%s)\n", structName, structFields[0].StructFile)
		for _, f := range structFields {
			tag := fmt.Sprintf("`json:\"%s,omitempty\"`", f.JSONKey)
			fmt.Fprintf(&b, "  + %s %s %s\n", f.GoFieldName, f.GoType, tag)
		}
	}

	return b.String()
}

// applyPatches applies the detected unknown fields to Go source files.
// It resolves struct file paths relative to the module root.
func applyPatches(fields []patch.UnknownField) error {
	// Find the module root by looking for go.mod from the current directory
	sourceDir, err := findModuleRoot()
	if err != nil {
		return fmt.Errorf("failed to find module root: %w", err)
	}

	fmt.Printf("\nApplying patches to source files in %s...\n", sourceDir)
	if err := patch.ApplyPatches(fields, sourceDir); err != nil {
		return fmt.Errorf("failed to apply patches: %w", err)
	}
	fmt.Println("Patches applied successfully.")
	return nil
}

// findModuleRoot walks up from the current directory to find go.mod.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent directory")
		}
		dir = parent
	}
}
