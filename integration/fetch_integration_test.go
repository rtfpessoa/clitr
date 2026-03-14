package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchIntegration_SaveAndLoadRoundTrip tests the full save/load cycle
// that would occur during a fetch operation. This tests the data persistence
// layer without requiring actual network connections.
func TestFetchIntegration_SaveAndLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sample raw events (simulating what would come from WebSocket)
	cursor := "eyJrZXlzZXQiOnsidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifSwicGFnZURpcmVjdGlvbiI6ImFmdGVyIn0="
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "order-001",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Apple Inc.",
				Status:    "EXECUTED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
				Amount:    &types.Amount{Value: -150.50, Currency: "EUR", FractionDigits: 2},
			},
			Details: types.TimelineDetails{
				ID: "order-001",
				Sections: []types.Section{
					{
						Title: "Transaction",
						Type:  "table",
						Data: []types.SectionDataItem{
							{Title: "Shares", Detail: types.ItemDetail{Text: "1.5"}},
							{Title: "Fee", Detail: types.ItemDetail{Text: "0.50 EUR"}},
						},
					},
				},
			},
			PageCursor: &cursor,
		},
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "order-002",
				Timestamp: "2024-02-20T14:45:00.000+0100",
				Title:     "SAP SE",
				Status:    "EXECUTED",
				EventType: "order_executed",
				Subtitle:  "Sell Order",
				Amount:    &types.Amount{Value: 250.75, Currency: "EUR", FractionDigits: 2},
			},
			Details: types.TimelineDetails{
				ID: "order-002",
				Sections: []types.Section{
					{
						Title: "Transaction",
						Type:  "table",
						Data: []types.SectionDataItem{
							{Title: "Shares", Detail: types.ItemDetail{Text: "2"}},
							{Title: "Taxes", Detail: types.ItemDetail{Text: "5.00 EUR"}},
						},
					},
				},
			},
			PageCursor: &cursor,
		},
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "pending-001",
				Timestamp: "2024-03-01T09:00:00.000+0100",
				Title:     "Pending Order",
				Status:    "PENDING",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
			},
			Details: types.TimelineDetails{
				ID:       "pending-001",
				Sections: []types.Section{},
			},
			PageCursor: &cursor,
		},
	}

	// Save raw events (simulating saveRawEvents from fetch.go)
	err := os.MkdirAll(tmpDir, 0755)
	require.NoError(t, err)

	for _, rawEvent := range rawEvents {
		rawEventData, err := json.Marshal(rawEvent)
		require.NoError(t, err)

		timestamp := rawEvent.TimelineEvent.Timestamp[:10]
		filename := timestamp + "_" + rawEvent.TimelineEvent.ID + ".json"
		filePath := filepath.Join(tmpDir, filename)

		err = os.WriteFile(filePath, rawEventData, 0644)
		require.NoError(t, err)
	}

	// Create metadata (simulating what saveMetadata does)
	metadata := map[string]interface{}{
		"last_fetch":    "2024-03-01T10:00:00Z",
		"event_count":   3,
		"pending_count": 1,
		"pending_items": []map[string]interface{}{
			{
				"event_id":  "pending-001",
				"timestamp": "2024-03-01T09:00:00Z",
			},
		},
	}
	metaData, err := json.Marshal(metadata)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.json"), metaData, 0644)
	require.NoError(t, err)

	// Now load the events back (simulating loadRawEventsFromDirectory from export.go)
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	var loadedRawEvents []*types.RawEvent
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		if file.Name() == "metadata.json" {
			continue
		}

		filePath := filepath.Join(tmpDir, file.Name())
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var rawEvent types.RawEvent
		err = json.Unmarshal(data, &rawEvent)
		require.NoError(t, err)
		loadedRawEvents = append(loadedRawEvents, &rawEvent)
	}

	// Verify we got all events back
	require.Len(t, loadedRawEvents, 3)

	// Verify data integrity
	eventsByID := make(map[string]*types.RawEvent)
	for _, re := range loadedRawEvents {
		eventsByID[re.TimelineEvent.ID] = re
	}

	// Check order-001
	order001 := eventsByID["order-001"]
	require.NotNil(t, order001)
	assert.Equal(t, "Apple Inc.", order001.TimelineEvent.Title)
	assert.Equal(t, "EXECUTED", order001.TimelineEvent.Status)
	assert.Equal(t, -150.50, order001.TimelineEvent.Amount.Value)
	assert.Len(t, order001.Details.Sections, 1)

	// Check order-002
	order002 := eventsByID["order-002"]
	require.NotNil(t, order002)
	assert.Equal(t, "SAP SE", order002.TimelineEvent.Title)
	assert.Equal(t, 250.75, order002.TimelineEvent.Amount.Value)

	// Check pending-001
	pending001 := eventsByID["pending-001"]
	require.NotNil(t, pending001)
	assert.Equal(t, "PENDING", pending001.TimelineEvent.Status)

	// Verify metadata was saved
	metaPath := filepath.Join(tmpDir, "metadata.json")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)

	metaContent, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var loadedMeta map[string]interface{}
	err = json.Unmarshal(metaContent, &loadedMeta)
	require.NoError(t, err)

	assert.Equal(t, float64(3), loadedMeta["event_count"])
	assert.Equal(t, float64(1), loadedMeta["pending_count"])
}

// TestFetchIntegration_IncrementalMode tests the metadata handling
// that supports incremental fetch mode
func TestFetchIntegration_IncrementalMode(t *testing.T) {
	tmpDir := t.TempDir()

	// First fetch: save 2 events with one pending
	cursor1 := "cursor1"
	rawEvents1 := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "event-001",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "First Event",
				Status:    "EXECUTED",
				EventType: "order_executed",
			},
			Details:    types.TimelineDetails{ID: "event-001"},
			PageCursor: &cursor1,
		},
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "event-002",
				Timestamp: "2024-01-16T10:30:00.000+0100",
				Title:     "Pending Event",
				Status:    "PENDING",
				EventType: "order_executed",
			},
			Details:    types.TimelineDetails{ID: "event-002"},
			PageCursor: &cursor1,
		},
	}

	// Save first batch
	for _, rawEvent := range rawEvents1 {
		rawEventData, err := json.Marshal(rawEvent)
		require.NoError(t, err)

		timestamp := rawEvent.TimelineEvent.Timestamp[:10]
		filename := timestamp + "_" + rawEvent.TimelineEvent.ID + ".json"
		filePath := filepath.Join(tmpDir, filename)

		err = os.WriteFile(filePath, rawEventData, 0644)
		require.NoError(t, err)
	}

	// Save metadata with pending item
	metadata1 := map[string]interface{}{
		"last_fetch":    "2024-01-16T12:00:00Z",
		"event_count":   2,
		"pending_count": 1,
		"pending_items": []map[string]interface{}{
			{
				"event_id":    "event-002",
				"timestamp":   "2024-01-16T10:30:00Z",
				"page_cursor": cursor1,
			},
		},
		"latest_page_cursor": cursor1,
	}
	metaData1, err := json.Marshal(metadata1)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.json"), metaData1, 0644)
	require.NoError(t, err)

	// Second fetch: the pending event is now executed, add a new event
	cursor2 := "cursor2"
	rawEvents2 := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "event-002", // Same ID, now executed
				Timestamp: "2024-01-16T10:30:00.000+0100",
				Title:     "Now Executed Event",
				Status:    "EXECUTED",
				EventType: "order_executed",
			},
			Details:    types.TimelineDetails{ID: "event-002"},
			PageCursor: &cursor2,
		},
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "event-003",
				Timestamp: "2024-01-17T10:30:00.000+0100",
				Title:     "New Event",
				Status:    "EXECUTED",
				EventType: "order_executed",
			},
			Details:    types.TimelineDetails{ID: "event-003"},
			PageCursor: &cursor2,
		},
	}

	// Save second batch (overwriting event-002)
	for _, rawEvent := range rawEvents2 {
		rawEventData, err := json.Marshal(rawEvent)
		require.NoError(t, err)

		timestamp := rawEvent.TimelineEvent.Timestamp[:10]
		filename := timestamp + "_" + rawEvent.TimelineEvent.ID + ".json"
		filePath := filepath.Join(tmpDir, filename)

		err = os.WriteFile(filePath, rawEventData, 0644)
		require.NoError(t, err)
	}

	// Update metadata
	metadata2 := map[string]interface{}{
		"last_fetch":         "2024-01-17T12:00:00Z",
		"event_count":        3,
		"pending_count":      0,
		"pending_items":      []map[string]interface{}{},
		"latest_page_cursor": cursor2,
	}
	metaData2, err := json.Marshal(metadata2)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.json"), metaData2, 0644)
	require.NoError(t, err)

	// Load all events
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	var loadedEvents []*types.RawEvent
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || file.Name() == "metadata.json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(tmpDir, file.Name()))
		require.NoError(t, err)

		var rawEvent types.RawEvent
		err = json.Unmarshal(data, &rawEvent)
		require.NoError(t, err)
		loadedEvents = append(loadedEvents, &rawEvent)
	}

	// Should have 3 unique events
	require.Len(t, loadedEvents, 3)

	// Verify event-002 was updated
	eventsByID := make(map[string]*types.RawEvent)
	for _, re := range loadedEvents {
		eventsByID[re.TimelineEvent.ID] = re
	}

	event002 := eventsByID["event-002"]
	require.NotNil(t, event002)
	assert.Equal(t, "EXECUTED", event002.TimelineEvent.Status)
	assert.Equal(t, "Now Executed Event", event002.TimelineEvent.Title)

	// Verify metadata shows no pending
	metaContent, err := os.ReadFile(filepath.Join(tmpDir, "metadata.json"))
	require.NoError(t, err)

	var loadedMeta map[string]interface{}
	err = json.Unmarshal(metaContent, &loadedMeta)
	require.NoError(t, err)

	assert.Equal(t, float64(0), loadedMeta["pending_count"])
}

// TestFetchIntegration_HandleReset tests the reset flag behavior
func TestFetchIntegration_HandleReset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	err := os.WriteFile(filepath.Join(tmpDir, "event1.json"), []byte("{}"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("{}"), 0644)
	require.NoError(t, err)

	// Verify files exist
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	// Simulate reset by removing directory
	err = os.RemoveAll(tmpDir)
	require.NoError(t, err)

	// Verify directory is gone
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err))

	// Recreate for fresh fetch
	err = os.MkdirAll(tmpDir, 0755)
	require.NoError(t, err)

	// Verify empty directory
	files, err = os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, files)
}
