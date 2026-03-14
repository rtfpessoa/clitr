package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rtfpessoa/clitr/internal/export"
	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/rtfpessoa/clitr/internal/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type exportConfig struct {
	*rootConfig

	outputFile string
	sortByDate bool
}

func NewExportCmd(rootConfig *rootConfig) *cobra.Command {
	exportConfig := &exportConfig{
		rootConfig: rootConfig,
	}

	// Export command - converts JSON data to CSV
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export transactions to CSV from saved data",
		Long:  "Convert previously fetched transaction data to CSV format",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, exportConfig)
		},
	}

	exportCmd.PersistentFlags().StringVarP(&exportConfig.outputFile, "output", "o", "transactions.csv", "Output CSV file path")
	exportCmd.PersistentFlags().BoolVarP(&exportConfig.sortByDate, "sort", "s", true, "Sort transactions chronologically")

	return exportCmd
}

func runExport(_ *cobra.Command, config *exportConfig) error {
	rawEvents, err := loadRawEventsFromDirectory(config.eventsDir)
	if err != nil {
		return err
	}

	events, err := parseRawEvents(rawEvents)
	if err != nil {
		return err
	}

	return exportToCSV(events, config.outputFile, config.sortByDate)
}

func loadRawEventsFromDirectory(dir string) ([]*types.RawEvent, error) {
	log.Info("Loading raw data", zap.String("dir", dir))

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list data files: %w", err)
	}

	rawEvents := make([]*types.RawEvent, 0, len(files))
	for _, file := range files {
		if filepath.Base(file) == "metadata.json" {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			log.Warn("Failed to read file", zap.String("file", file), zap.Error(err))
			continue
		}

		var rawEvent types.RawEvent
		if err := json.Unmarshal(data, &rawEvent); err != nil {
			log.Warn("Failed to parse file", zap.String("file", file), zap.Error(err))
			continue
		}

		rawEvents = append(rawEvents, &rawEvent)
	}

	if len(rawEvents) == 0 {
		return nil, fmt.Errorf("no raw transaction data found in %s. Run 'fetch' command first", dir)
	}

	log.Info("Loaded raw transactions", zap.Int("count", len(rawEvents)))
	return rawEvents, nil
}

func parseRawEvents(rawEvents []*types.RawEvent) ([]*types.Event, error) {
	log.Info("Parsing transactions")
	events := make([]*types.Event, 0, len(rawEvents))

	for _, rawEvent := range rawEvents {
		event, err := types.ParseEvent(rawEvent.TimelineEvent, rawEvent.Details)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		if event != nil {
			events = append(events, event)
		}
	}

	log.Info("Parsed transactions", zap.Int("count", len(events)))
	return events, nil
}

func exportToCSV(events []*types.Event, outputFile string, sortByDate bool) error {
	log.Info("Exporting to CSV", zap.String("file", outputFile))

	outputFile, err := utils.ResolvePath(outputFile)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error("failed to close file", zap.Error(err))
		}
	}(file)

	exporter := export.NewCSVExporter(file)
	if err := exporter.Export(events, sortByDate); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	log.Info("Successfully exported transactions",
		zap.Int("count", len(events)),
		zap.String("file", outputFile))
	return nil
}
