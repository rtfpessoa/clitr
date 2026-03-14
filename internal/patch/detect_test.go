package patch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectUnknownFields_NoUnknowns(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED",
			"eventType": "payment_inbound",
			"subtitle": "sub"
		},
		"details": {
			"id": "123",
			"sections": []
		}
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	assert.Empty(t, fields)
}

func TestDetectUnknownFields_UnknownInTimelineEvent(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED",
			"eventType": "payment_inbound",
			"subtitle": "sub",
			"newStringField": "hello",
			"newBoolField": true
		},
		"details": {
			"id": "123",
			"sections": []
		}
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 2)

	fieldMap := make(map[string]UnknownField)
	for _, f := range fields {
		fieldMap[f.JSONKey] = f
	}

	assert.Equal(t, "TimelineEvent", fieldMap["newStringField"].StructName)
	assert.Equal(t, "string", fieldMap["newStringField"].GoType)
	assert.Equal(t, "NewStringField", fieldMap["newStringField"].GoFieldName)
	assert.Equal(t, "internal/types/event.go", fieldMap["newStringField"].StructFile)

	assert.Equal(t, "TimelineEvent", fieldMap["newBoolField"].StructName)
	assert.Equal(t, "bool", fieldMap["newBoolField"].GoType)
}

func TestDetectUnknownFields_UnknownInSection(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED"
		},
		"details": {
			"id": "123",
			"sections": [
				{
					"title": "Overview",
					"type": "table",
					"unknownSectionField": 42
				}
			]
		}
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 1)

	assert.Equal(t, "Section", fields[0].StructName)
	assert.Equal(t, "unknownSectionField", fields[0].JSONKey)
	assert.Equal(t, "float64", fields[0].GoType)
	assert.Equal(t, "internal/types/raw.go", fields[0].StructFile)
}

func TestDetectUnknownFields_UnknownInNestedSectionDataItem(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED"
		},
		"details": {
			"id": "123",
			"sections": [
				{
					"title": "Overview",
					"type": "table",
					"data": [
						{
							"title": "Amount",
							"detail": {"text": "100 EUR"},
							"newDataField": "extra"
						}
					]
				}
			]
		}
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 1)

	assert.Equal(t, "SectionDataItem", fields[0].StructName)
	assert.Equal(t, "newDataField", fields[0].JSONKey)
	assert.Equal(t, "string", fields[0].GoType)
}

func TestDetectUnknownFields_UnknownInRawEvent(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED"
		},
		"details": {
			"id": "123",
			"sections": []
		},
		"pageCursor": "abc",
		"newTopLevelField": "value"
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 1)

	assert.Equal(t, "RawEvent", fields[0].StructName)
	assert.Equal(t, "newTopLevelField", fields[0].JSONKey)
}

func TestDetectUnknownFields_MultipleNestingLevels(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED",
			"unknownTE": "a"
		},
		"details": {
			"id": "123",
			"sections": [
				{
					"title": "Overview",
					"unknownSection": "b",
					"data": [
						{
							"title": "Item",
							"detail": {"text": "val", "unknownDetail": "c"}
						}
					]
				}
			]
		},
		"unknownRoot": "d"
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 4)

	structNames := make(map[string]bool)
	for _, f := range fields {
		structNames[f.StructName] = true
	}

	assert.True(t, structNames["RawEvent"])
	assert.True(t, structNames["TimelineEvent"])
	assert.True(t, structNames["Section"])
	assert.True(t, structNames["ItemDetail"])
}

func TestDetectUnknownFields_UnknownInTimelineDetails(t *testing.T) {
	jsonData := []byte(`{
		"timelineEvent": {
			"id": "123",
			"timestamp": "2024-01-01T00:00:00.000+0000",
			"icon": "icon",
			"title": "Test",
			"body": "body",
			"status": "EXECUTED"
		},
		"details": {
			"id": "123",
			"sections": [],
			"unknownDetailsField": true
		}
	}`)

	fields, err := DetectUnknownFields(jsonData)
	require.NoError(t, err)
	require.Len(t, fields, 1)

	assert.Equal(t, "TimelineDetails", fields[0].StructName)
	assert.Equal(t, "unknownDetailsField", fields[0].JSONKey)
	assert.Equal(t, "internal/types/raw.go", fields[0].StructFile)
}

func TestScanDirectory(t *testing.T) {
	dir := t.TempDir()

	// Write two JSON files with overlapping unknown fields
	file1 := []byte(`{
		"timelineEvent": {"id":"1","timestamp":"2024-01-01T00:00:00.000+0000","icon":"i","title":"t","body":"b","status":"EXECUTED"},
		"details": {"id":"1","sections":[]},
		"newTopField": "a"
	}`)
	file2 := []byte(`{
		"timelineEvent": {"id":"2","timestamp":"2024-01-01T00:00:00.000+0000","icon":"i","title":"t","body":"b","status":"EXECUTED","newTEField": true},
		"details": {"id":"2","sections":[]},
		"newTopField": "b"
	}`)
	// Metadata file should be skipped
	metadata := []byte(`{"last_fetch":"2024-01-01"}`)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "2024-01-01_event1.json"), file1, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2024-01-01_event2.json"), file2, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0644))

	fields, err := ScanDirectory(dir)
	require.NoError(t, err)

	// Should find 2 unique unknown fields (newTopField deduplicated, plus newTEField)
	require.Len(t, fields, 2)

	fieldMap := make(map[string]UnknownField)
	for _, f := range fields {
		fieldMap[f.JSONKey] = f
	}

	assert.Equal(t, "RawEvent", fieldMap["newTopField"].StructName)
	assert.Equal(t, "TimelineEvent", fieldMap["newTEField"].StructName)
}

func TestScanDirectory_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	fields, err := ScanDirectory(dir)
	require.NoError(t, err)
	assert.Empty(t, fields)
}
