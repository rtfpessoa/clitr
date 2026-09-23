package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"go.uber.org/zap"
)

func saveRawEvents(rawEvents []*rawEventFile, dir string) error {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	log.Info("Saving raw data", zap.String("dir", dir))

	// Save each raw event to individual JSON file
	for _, rawEvent := range rawEvents {
		rawEventData, err := json.Marshal(rawEvent)
		if err != nil {
			return fmt.Errorf("failed to marshal raw event: %w", err)
		}

		eventMap, _ := rawEvent.TimelineEvent.(map[string]interface{})
		timestamp, _ := eventMap["timestamp"].(string)
		id, _ := eventMap["id"].(string)

		if len(timestamp) < 10 || id == "" {
			log.Warn("Skipping event with missing id or timestamp", zap.String("id", id), zap.String("timestamp", timestamp))
			continue
		}

		date := timestamp[:10]
		filename := fmt.Sprintf("%s_%s.json", date, id)
		filePath := filepath.Join(dir, filename)

		if err := os.WriteFile(filePath, rawEventData, 0644); err != nil {
			return fmt.Errorf("failed to write raw event file: %w", err)
		}
	}

	// Save metadata
	if err := saveMetadata(rawEvents, dir); err != nil {
		return err
	}

	log.Info("Successfully saved transactions",
		zap.Int("count", len(rawEvents)),
		zap.String("dir", dir))
	return nil
}

type pendingItem struct {
	EventID    string    `json:"event_id"`
	Timestamp  time.Time `json:"timestamp"`
	PageCursor *string   `json:"page_cursor"`
}

type metadata struct {
	LastFetch time.Time `json:"last_fetch"`

	EventCount           int       `json:"event_count"`
	LatestEventTimestamp time.Time `json:"latest_event_ts"`

	PendingCount int           `json:"pending_count"`
	PendingItems []pendingItem `json:"pending_items"`

	LatestPageCursor *string `json:"latest_page_cursor"`
}

func saveMetadata(rawEvents []*rawEventFile, dir string) error {
	var pendingItems []pendingItem

	var latestTimestamp time.Time
	var latestCursor *string

	for _, rawEvent := range rawEvents {
		eventMap, _ := rawEvent.TimelineEvent.(map[string]interface{})
		status, _ := eventMap["status"].(string)
		timestampStr, _ := eventMap["timestamp"].(string)
		id, _ := eventMap["id"].(string)

		timestamp, err := time.Parse(types.TimestampLayout, timestampStr)
		if err != nil {
			log.Warn("Failed to parse timestamp for metadata", zap.String("id", id), zap.String("timestamp", timestampStr), zap.Error(err))
			continue
		}

		statusUpper := strings.ToUpper(status)
		if statusUpper != "EXECUTED" && statusUpper != "CANCELED" {
			pendingItems = append(pendingItems, pendingItem{
				EventID:    id,
				Timestamp:  timestamp,
				PageCursor: rawEvent.PageCursor,
			})
		}

		if timestamp.After(latestTimestamp) {
			latestTimestamp = timestamp
			latestCursor = rawEvent.PageCursor
		}
	}

	meta := metadata{
		LastFetch: time.Now(),

		EventCount: len(rawEvents),

		PendingCount: len(pendingItems),
		PendingItems: pendingItems,

		LatestEventTimestamp: latestTimestamp,
		LatestPageCursor:     latestCursor,
	}

	metadataBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), metadataBytes, 0644); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	return nil
}

func loadMetadata(dataDir string) (*metadata, error) {
	metadataPath := filepath.Join(dataDir, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &meta, nil
}
