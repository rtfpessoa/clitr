package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRawEventsFromDirectory_ValidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid event file
	eventJSON := `{
		"timelineEvent": {
			"id": "test-1",
			"timestamp": "2024-01-15T10:30:00.000+0100",
			"title": "Test Event",
			"status": "EXECUTED",
			"eventType": "order_executed",
			"subtitle": "Buy Order"
		},
		"details": {"id": "test-1", "sections": []}
	}`
	err := os.WriteFile(filepath.Join(tmpDir, "2024-01-15_test-1.json"), []byte(eventJSON), 0644)
	require.NoError(t, err)

	rawEvents, err := loadRawEventsFromDirectory(tmpDir)
	require.NoError(t, err)
	require.Len(t, rawEvents, 1)

	assert.Equal(t, "test-1", rawEvents[0].TimelineEvent.ID)
}

func TestLoadRawEventsFromDirectory_SkipMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid event file
	eventJSON := `{
		"timelineEvent": {
			"id": "test-1",
			"timestamp": "2024-01-15T10:30:00.000+0100",
			"title": "Test Event",
			"status": "EXECUTED",
			"eventType": "order_executed",
			"subtitle": "Buy Order"
		},
		"details": {"id": "test-1", "sections": []}
	}`
	err := os.WriteFile(filepath.Join(tmpDir, "2024-01-15_test-1.json"), []byte(eventJSON), 0644)
	require.NoError(t, err)

	// Create metadata.json which should be skipped
	metaJSON := `{"last_fetch": "2024-01-15T10:30:00Z", "event_count": 1}`
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metaJSON), 0644)
	require.NoError(t, err)

	rawEvents, err := loadRawEventsFromDirectory(tmpDir)
	require.NoError(t, err)
	require.Len(t, rawEvents, 1) // Only 1 event, metadata skipped
}

func TestParseRawEvents_ValidEvents(t *testing.T) {
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "test-1",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Apple Inc.",
				Status:    "EXECUTED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
				Amount:    &types.Amount{Value: -100.0, Currency: "EUR"},
			},
			Details: types.TimelineDetails{
				ID:       "test-1",
				Sections: []types.Section{},
			},
		},
	}

	events, err := parseRawEvents(rawEvents)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, "test-1", events[0].ID)
	assert.Equal(t, types.EventTypeTradeInvoice, events[0].EventType)
}

func TestParseRawEvents_SkipCanceled(t *testing.T) {
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "canceled-1",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Canceled Order",
				Status:    "CANCELED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
			},
			Details: types.TimelineDetails{ID: "canceled-1"},
		},
	}

	events, err := parseRawEvents(rawEvents)
	require.NoError(t, err)
	assert.Empty(t, events) // Canceled events are skipped
}

func TestExportToCSV_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.csv")

	val := -100.0
	events := []*types.Event{
		{
			ID:        "test-1",
			Title:     "Test Event",
			EventType: types.EventTypeTradeInvoice,
			Value:     &val,
		},
	}

	err := exportToCSV(events, outputFile, true)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(outputFile)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Date;Type;Value")
}

func TestExportToCSV_ResolvesPath(t *testing.T) {
	// Use an absolute path (TempDir already returns absolute path)
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "subdir", "output.csv")

	// Create the subdir
	err := os.MkdirAll(filepath.Dir(outputFile), 0755)
	require.NoError(t, err)

	events := []*types.Event{}

	err = exportToCSV(events, outputFile, false)
	require.NoError(t, err)

	// Verify file was created at the resolved path
	_, err = os.Stat(outputFile)
	require.NoError(t, err)
}
