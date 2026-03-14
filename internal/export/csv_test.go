package export

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExport_SortsByDate(t *testing.T) {
	var buf bytes.Buffer
	exporter := NewCSVExporter(&buf)

	val1 := -100.0
	val2 := -200.0

	events := []*types.Event{
		{
			ID:        "2",
			Date:      time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Title:     "Second",
			EventType: types.EventTypeTradeInvoice,
			Value:     &val2,
		},
		{
			ID:        "1",
			Date:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Title:     "First",
			EventType: types.EventTypeTradeInvoice,
			Value:     &val1,
		},
	}

	err := exporter.Export(events, true)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 3) // header + 2 rows

	// First event should be January (sorted)
	assert.Contains(t, lines[1], "2024-01-01")
	assert.Contains(t, lines[2], "2024-02-01")
}

func TestExport_WritesHeader(t *testing.T) {
	var buf bytes.Buffer
	exporter := NewCSVExporter(&buf)

	err := exporter.Export([]*types.Event{}, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Date;Type;Value;Note;ISIN;Shares;Fees;Taxes")
}

func TestEventToRows_StandardEvent(t *testing.T) {
	exporter := NewCSVExporter(nil)

	val := -100.0
	shares := 5.0
	fees := 1.0

	event := &types.Event{
		Date:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Title:     "Apple Inc.",
		EventType: types.EventTypeTradeInvoice,
		ISIN:      "US0378331005",
		Value:     &val,
		Shares:    &shares,
		Fees:      &fees,
	}

	rows := exporter.eventToRows(event)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "2024-01-15T10:30:00Z", row[0]) // Date
	assert.Equal(t, "BUY", row[1])                  // Type (negative value = BUY)
	assert.Equal(t, "-100.00", row[2])              // Value
	assert.Equal(t, "US0378331005", row[4])         // ISIN
	assert.Equal(t, "5.00", row[5])                 // Shares
	assert.Equal(t, "-1.00", row[6])                // Fees (negated)
}

func TestEventToRows_SavebackDualRow(t *testing.T) {
	exporter := NewCSVExporter(nil)

	val := 1.50
	shares := 0.018

	event := &types.Event{
		Date:      time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC),
		Title:     "Saveback",
		EventType: types.EventTypeSaveback,
		ISIN:      "IE00B4L5Y983",
		Value:     &val,
		Shares:    &shares,
	}

	rows := exporter.eventToRows(event)
	require.Len(t, rows, 2)

	// First row is BUY
	assert.Equal(t, "BUY", rows[0][1])
	assert.Equal(t, "1.50", rows[0][2])
	assert.Equal(t, "IE00B4L5Y983", rows[0][4])
	assert.Equal(t, "0.02", rows[0][5]) // Shares formatted

	// Second row is DEPOSIT with negated value
	assert.Equal(t, "DEPOSIT", rows[1][1])
	assert.Equal(t, "-1.50", rows[1][2]) // Negated value
	assert.Equal(t, "", rows[1][4])      // No ISIN
	assert.Equal(t, "", rows[1][5])      // No shares
}

func TestFormatFloat_NilValue(t *testing.T) {
	result := formatFloat(nil)
	assert.Equal(t, "", result)
}

func TestFormatFloat_ValidValue(t *testing.T) {
	val := 123.456
	result := formatFloat(&val)
	assert.Equal(t, "123.46", result)
}
