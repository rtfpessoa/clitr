package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimelineMessage_ValidItems(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"id":        "event-1",
					"timestamp": "2024-01-15T10:30:00.000+0100",
					"title":     "Apple Inc.",
					"status":    "EXECUTED",
				},
			},
		},
	}

	items, cursor, err := parseTimelineMessage(msg)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "event-1", items[0]["id"])
	assert.Equal(t, "Apple Inc.", items[0]["title"])
	assert.NotNil(t, cursor)
}

func TestParseTimelineMessage_WithCursors(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{},
			"cursors": map[string]interface{}{
				"before": "cursor-before",
				"after":  "cursor-after",
			},
		},
	}

	_, cursor, err := parseTimelineMessage(msg)
	require.NoError(t, err)
	require.NotNil(t, cursor)

	assert.Equal(t, "cursor-before", *cursor.Before)
	assert.Equal(t, "cursor-after", *cursor.After)
}

func TestParseTimelineMessage_PreservesUnknownFields(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"id":           "event-1",
					"timestamp":    "2024-01-15T10:30:00.000+0100",
					"title":        "Apple Inc.",
					"status":       "EXECUTED",
					"unknownField": "preserved",
				},
			},
		},
	}

	items, _, err := parseTimelineMessage(msg)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Unknown fields are preserved in the raw map
	assert.Equal(t, "preserved", items[0]["unknownField"])
}

func TestChangeCursor_DirectionChange(t *testing.T) {
	// Create a valid base64-encoded cursor with proper structure
	cursorJSON := `{"keyset":{"eventId":"test-123","timestamp":"2024-01-15T10:30:00Z"},"pageDirection":"after"}`
	encoded := base64.RawStdEncoding.EncodeToString([]byte(cursorJSON))

	result := changeCursor(&encoded, FetchDirectionBefore)
	require.NotNil(t, result)

	// Decode the result to verify direction changed
	decoded, err := base64.RawStdEncoding.DecodeString(*result)
	require.NoError(t, err)
	// JSON is pretty-printed, so check for the value anywhere in the output
	assert.Contains(t, string(decoded), `"pageDirection": "before"`)
}

func TestChangeCursor_NilInput(t *testing.T) {
	result := changeCursor(nil, FetchDirectionAfter)
	assert.Nil(t, result)
}

func TestSaveMetadata_WithPendingItems(t *testing.T) {
	tmpDir := t.TempDir()

	rawEvents := []*rawEventFile{
		{
			TimelineEvent: map[string]interface{}{
				"id":        "pending-1",
				"timestamp": "2024-01-15T10:30:00.000+0100",
				"status":    "PENDING",
			},
		},
		{
			TimelineEvent: map[string]interface{}{
				"id":        "executed-1",
				"timestamp": "2024-01-16T10:30:00.000+0100",
				"status":    "EXECUTED",
			},
		},
	}

	err := saveMetadata(rawEvents, tmpDir)
	require.NoError(t, err)

	// Verify metadata file was created
	metaPath := filepath.Join(tmpDir, "metadata.json")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)

	// Load and verify
	meta, err := loadMetadata(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, 2, meta.EventCount)
	assert.Equal(t, 1, meta.PendingCount)
	assert.Len(t, meta.PendingItems, 1)
	assert.Equal(t, "pending-1", meta.PendingItems[0].EventID)
}

func TestSaveMetadata_AllExecuted(t *testing.T) {
	tmpDir := t.TempDir()

	rawEvents := []*rawEventFile{
		{
			TimelineEvent: map[string]interface{}{
				"id":        "executed-1",
				"timestamp": "2024-01-15T10:30:00.000+0100",
				"status":    "EXECUTED",
			},
		},
	}

	err := saveMetadata(rawEvents, tmpDir)
	require.NoError(t, err)

	meta, err := loadMetadata(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, 0, meta.PendingCount)
	assert.Empty(t, meta.PendingItems)
}

func TestLoadMetadata_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid metadata file
	metaJSON := `{
		"last_fetch": "2024-01-15T10:30:00Z",
		"event_count": 5,
		"pending_count": 1,
		"pending_items": [{"event_id": "test-1", "timestamp": "2024-01-15T10:30:00Z"}]
	}`
	err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metaJSON), 0644)
	require.NoError(t, err)

	meta, err := loadMetadata(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, 5, meta.EventCount)
	assert.Equal(t, 1, meta.PendingCount)
}

func TestLoadMetadata_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := loadMetadata(tmpDir)
	require.NoError(t, err)
	assert.Nil(t, meta)
}

func TestHandleResetData_Reset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = handleResetData(true, tmpDir)
	require.NoError(t, err)

	// Directory should be removed
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err))
}

func TestHandleResetData_NoReset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = handleResetData(false, tmpDir)
	require.NoError(t, err)

	// Directory should still exist
	_, err = os.Stat(tmpDir)
	require.NoError(t, err)

	// File should still exist
	_, err = os.Stat(testFile)
	require.NoError(t, err)
}

func TestSaveRawEvents_CreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()

	rawEvents := []*rawEventFile{
		{
			TimelineEvent: map[string]interface{}{
				"id":        "event-1",
				"timestamp": "2024-01-15T10:30:00.000+0100",
				"title":     "Test Event",
				"status":    "EXECUTED",
			},
			Details: map[string]interface{}{
				"id":       "event-1",
				"sections": []interface{}{},
			},
		},
	}

	err := saveRawEvents(rawEvents, tmpDir)
	require.NoError(t, err)

	// Verify event file was created
	expectedFile := filepath.Join(tmpDir, "2024-01-15_event-1.json")
	_, err = os.Stat(expectedFile)
	require.NoError(t, err)

	// Verify metadata was created
	metaFile := filepath.Join(tmpDir, "metadata.json")
	_, err = os.Stat(metaFile)
	require.NoError(t, err)
}

func TestSaveRawEvents_PreservesUnknownFields(t *testing.T) {
	tmpDir := t.TempDir()

	rawEvents := []*rawEventFile{
		{
			TimelineEvent: map[string]interface{}{
				"id":           "event-1",
				"timestamp":    "2024-01-15T10:30:00.000+0100",
				"title":        "Test Event",
				"status":       "EXECUTED",
				"unknownField": "should be preserved",
			},
			Details: map[string]interface{}{
				"id":           "event-1",
				"sections":     []interface{}{},
				"newDetailKey": true,
			},
		},
	}

	err := saveRawEvents(rawEvents, tmpDir)
	require.NoError(t, err)

	// Read the saved file and verify unknown fields are preserved
	data, err := os.ReadFile(filepath.Join(tmpDir, "2024-01-15_event-1.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "unknownField")
	assert.Contains(t, string(data), "should be preserved")
	assert.Contains(t, string(data), "newDetailKey")
}

func TestAssembleRawEvents(t *testing.T) {
	items := []map[string]interface{}{
		{"id": "e1", "title": "Event 1"},
		{"id": "e2", "title": "Event 2"},
	}
	details := map[string]map[string]interface{}{
		"e1": {"id": "e1", "sections": []interface{}{}},
	}
	cursor := "test-cursor"

	result := assembleRawEvents(items, details, &cursor)
	require.Len(t, result, 2)

	assert.Equal(t, items[0], result[0].TimelineEvent)
	assert.Equal(t, details["e1"], result[0].Details)
	assert.Equal(t, &cursor, result[0].PageCursor)

	// e2 has no detail
	assert.Nil(t, result[1].Details)
}

// Helper to ensure time.Time can be used in assertions
func init() {
	_ = time.Now()
}
