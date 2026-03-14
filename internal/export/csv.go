// Package export provides exporters for converting Trade Republic transaction data
// into various output formats, including CSV.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"

	"github.com/rtfpessoa/clitr/internal/types"
)

// CSVExporter handles exporting events to CSV format
type CSVExporter struct {
	writer *csv.Writer
}

// NewCSVExporter creates a new CSV exporter
func NewCSVExporter(w io.Writer) *CSVExporter {
	csvWriter := csv.NewWriter(w)
	csvWriter.Comma = ';'
	return &CSVExporter{
		writer: csvWriter,
	}
}

// Export writes events to CSV
func (e *CSVExporter) Export(events []*types.Event, sortByDate bool) error {
	// Sort events by date if requested
	if sortByDate {
		sort.Slice(events, func(i, j int) bool {
			return events[i].Date.Before(events[j].Date)
		})
	}

	// Write header
	header := []string{"Date", "Type", "Value", "Note", "ISIN", "Shares", "Fees", "Taxes"}
	if err := e.writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write events
	for _, event := range events {
		rows := e.eventToRows(event)
		for _, row := range rows {
			if err := e.writer.Write(row); err != nil {
				return fmt.Errorf("failed to write row: %w", err)
			}
		}
	}

	e.writer.Flush()
	return e.writer.Error()
}

// eventToRows converts an event to one or more CSV rows
func (e *CSVExporter) eventToRows(event *types.Event) [][]string {
	var rows [][]string

	transType := event.GetTransactionType()

	// Handle special cases that generate multiple rows
	if event.EventType == types.EventTypeSaveback {
		// Saveback generates a BUY and a DEPOSIT
		buyRow := []string{
			event.Date.Format("2006-01-02T15:04:05Z07:00"),
			string(types.EventTypeBuy),
			formatFloat(event.Value),
			event.GetNote(),
			event.ISIN,
			formatFloat(event.Shares),
			formatFloat(negateFloat(event.Fees)),
			formatFloat(negateFloat(event.Taxes)),
		}
		rows = append(rows, buyRow)

		// Add deposit row (negated value, no ISIN/shares)
		depositRow := []string{
			event.Date.Format("2006-01-02T15:04:05Z07:00"),
			string(types.EventTypeDeposit),
			formatFloat(negateFloat(event.Value)),
			event.GetNote(),
			"",
			"",
			"",
			"",
		}
		rows = append(rows, depositRow)
	} else {
		// Standard single row
		row := []string{
			event.Date.Format("2006-01-02T15:04:05Z07:00"),
			string(transType),
			formatFloat(event.Value),
			event.GetNote(),
			event.ISIN,
			formatFloat(event.Shares),
			formatFloat(negateFloat(event.Fees)),
			formatFloat(negateFloat(event.Taxes)),
		}
		rows = append(rows, row)
	}

	return rows
}

// formatFloat formats a float pointer to string, or empty string if nil
func formatFloat(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *f)
}

// negateFloat returns a new pointer to negated value, or nil if input is nil
func negateFloat(f *float64) *float64 {
	if f == nil {
		return nil
	}
	negated := -*f
	return &negated
}
