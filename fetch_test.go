package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rtfpessoa/clitr/internal/patch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOldestPendingCursor(t *testing.T) {
	newer, older := "newer", "older"
	items := []pendingItem{
		{Timestamp: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), PageCursor: &newer},
		{Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), PageCursor: &older},
	}
	cursor, err := oldestPendingCursor(items)
	require.NoError(t, err)
	assert.Equal(t, &older, cursor)
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

func TestConvertMapsToRawEventFiles(t *testing.T) {
	cursor := "fallback-cursor"
	rawMaps := []map[string]interface{}{
		{
			"timelineEvent": map[string]interface{}{
				"id":        "event-1",
				"timestamp": "2024-01-15T10:30:00.000+0100",
				"status":    "EXECUTED",
			},
			"details": map[string]interface{}{
				"id":       "event-1",
				"sections": []interface{}{},
			},
		},
	}

	result := convertMapsToRawEventFiles(rawMaps, &cursor)
	require.Len(t, result, 1)
	assert.Equal(t, &cursor, result[0].PageCursor)
	assert.NotNil(t, result[0].TimelineEvent)
	assert.NotNil(t, result[0].Details)
}

func TestConvertMapsToRawEventFiles_Empty(t *testing.T) {
	cursor := "test-cursor"
	result := convertMapsToRawEventFiles(nil, &cursor)
	assert.Empty(t, result)
}

func TestConvertMapsToRawEventFiles_PreservesUnknownFields(t *testing.T) {
	rawMaps := []map[string]interface{}{
		{
			"timelineEvent": map[string]interface{}{
				"id":           "event-1",
				"timestamp":    "2024-01-15T10:30:00.000+0100",
				"status":       "EXECUTED",
				"unknownField": "preserved",
			},
			"details": map[string]interface{}{
				"id":           "event-1",
				"sections":     []interface{}{},
				"newDetailKey": true,
			},
		},
	}

	result := convertMapsToRawEventFiles(rawMaps, nil)
	require.Len(t, result, 1)

	// Verify unknown fields survive in the rawEventFile
	eventMap := result[0].TimelineEvent.(map[string]interface{})
	assert.Equal(t, "preserved", eventMap["unknownField"])

	detailMap := result[0].Details.(map[string]interface{})
	assert.Equal(t, true, detailMap["newDetailKey"])
}

// TestEndToEnd_UnknownFieldsSurviveSaveToPatchDetect verifies the full pipeline:
// raw maps with unknown fields → save to disk → patch.DetectUnknownFields finds them.
func TestEndToEnd_UnknownFieldsSurviveSaveToPatchDetect(t *testing.T) {
	tmpDir := t.TempDir()

	rawMaps := []map[string]interface{}{
		{
			"timelineEvent": map[string]interface{}{
				"id":             "event-1",
				"timestamp":      "2024-01-15T10:30:00.000+0100",
				"title":          "Test Event",
				"status":         "EXECUTED",
				"eventType":      "order_executed",
				"brandNewAPIKey": "surprise",
			},
			"details": map[string]interface{}{
				"id":       "event-1",
				"sections": []interface{}{},
			},
		},
	}

	rawEventFiles := convertMapsToRawEventFiles(rawMaps, nil)
	err := saveRawEvents(rawEventFiles, tmpDir)
	require.NoError(t, err)

	// Now scan with patch detection
	fields, err := patch.ScanDirectory(tmpDir)
	require.NoError(t, err)
	require.NotEmpty(t, fields, "patch should detect unknown fields in saved files")

	// Find the specific unknown field
	found := false
	for _, f := range fields {
		if f.JSONKey == "brandNewAPIKey" {
			found = true
			assert.Equal(t, "TimelineEvent", f.StructName)
			break
		}
	}
	assert.True(t, found, "should detect 'brandNewAPIKey' as unknown field in TimelineEvent")
}

// Helper to ensure time.Time can be used in assertions
func init() {
	_ = time.Now()
}
