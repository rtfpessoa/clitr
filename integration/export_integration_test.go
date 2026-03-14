package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rtfpessoa/clitr/internal/export"
	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportIntegration_FullFlow tests the full export flow from JSON files to CSV
func TestExportIntegration_FullFlow(t *testing.T) {
	// Create temp directory for output
	tmpDir := t.TempDir()

	// Load sample events from testdata
	testdataDir := filepath.Join("..", "testdata", "events")

	// Read buy_order.json
	buyOrderData, err := os.ReadFile(filepath.Join(testdataDir, "buy_order.json"))
	require.NoError(t, err, "Failed to read buy_order.json")

	var buyOrderRaw types.RawEvent
	err = json.Unmarshal(buyOrderData, &buyOrderRaw)
	require.NoError(t, err, "Failed to parse buy_order.json")

	// Read sell_order.json
	sellOrderData, err := os.ReadFile(filepath.Join(testdataDir, "sell_order.json"))
	require.NoError(t, err, "Failed to read sell_order.json")

	var sellOrderRaw types.RawEvent
	err = json.Unmarshal(sellOrderData, &sellOrderRaw)
	require.NoError(t, err, "Failed to parse sell_order.json")

	// Read dividend.json
	dividendData, err := os.ReadFile(filepath.Join(testdataDir, "dividend.json"))
	require.NoError(t, err, "Failed to read dividend.json")

	var dividendRaw types.RawEvent
	err = json.Unmarshal(dividendData, &dividendRaw)
	require.NoError(t, err, "Failed to parse dividend.json")

	// Read canceled.json
	canceledData, err := os.ReadFile(filepath.Join(testdataDir, "canceled.json"))
	require.NoError(t, err, "Failed to read canceled.json")

	var canceledRaw types.RawEvent
	err = json.Unmarshal(canceledData, &canceledRaw)
	require.NoError(t, err, "Failed to parse canceled.json")

	// Parse all raw events
	rawEvents := []*types.RawEvent{&buyOrderRaw, &sellOrderRaw, &dividendRaw, &canceledRaw}

	var events []*types.Event
	for _, rawEvent := range rawEvents {
		event, err := types.ParseEvent(rawEvent.TimelineEvent, rawEvent.Details)
		require.NoError(t, err, "Failed to parse event")
		if event != nil {
			events = append(events, event)
		}
	}

	// Should have 3 events (canceled one is skipped)
	require.Len(t, events, 3, "Expected 3 events (canceled should be skipped)")

	// Export to CSV
	outputFile := filepath.Join(tmpDir, "transactions.csv")
	file, err := os.Create(outputFile)
	require.NoError(t, err)

	exporter := export.NewCSVExporter(file)
	err = exporter.Export(events, true)
	require.NoError(t, err)
	file.Close()

	// Read and verify CSV content
	csvContent, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	csvString := string(csvContent)

	// Verify header
	assert.Contains(t, csvString, "Date;Type;Value;Note;ISIN;Shares;Fees;Taxes")

	// Verify we have the right number of lines (header + 3 events)
	lines := strings.Split(strings.TrimSpace(csvString), "\n")
	assert.Len(t, lines, 4, "Expected 4 lines: header + 3 events")

	// Verify events are sorted by date (earliest first)
	// Buy order: 2024-01-15
	// Sell order: 2024-02-20
	// Dividend: 2024-03-15
	assert.Contains(t, lines[1], "2024-01-15")
	assert.Contains(t, lines[2], "2024-02-20")
	assert.Contains(t, lines[3], "2024-03-15")

	// Verify event types
	assert.Contains(t, lines[1], "BUY")
	assert.Contains(t, lines[2], "SELL")
	assert.Contains(t, lines[3], "DIVIDEND")

	// Verify ISINs are present
	assert.Contains(t, lines[1], "US0378331005") // Apple
	assert.Contains(t, lines[2], "DE0007164600") // SAP
	assert.Contains(t, lines[3], "US5949181045") // Microsoft
}
