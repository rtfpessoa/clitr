// Package fetch provides the transaction fetching pipeline.
package fetch

import (
	"context"
	stdjson "encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"go.uber.org/zap"
)

// ParseRawMaps converts raw combined maps (from FetchAllEvents) into typed RawEvent objects.
// Uses stdlib encoding/json to tolerate unknown API fields that aren't yet in the Go structs.
func ParseRawMaps(rawMaps []map[string]interface{}) ([]*types.RawEvent, error) {
	events := make([]*types.RawEvent, 0, len(rawMaps))
	for _, m := range rawMaps {
		jsonBytes, err := stdjson.Marshal(m)
		if err != nil {
			log.Warn("Failed to marshal raw map", zap.Error(err))
			continue
		}
		var rawEvent types.RawEvent
		if err := stdjson.Unmarshal(jsonBytes, &rawEvent); err != nil {
			log.Warn("Failed to unmarshal raw map to RawEvent", zap.Error(err))
			continue
		}
		events = append(events, &rawEvent)
	}
	return events, nil
}

// ProgressFunc is called after each page of timeline events is fetched.
// page is the 1-based page number, eventsSoFar is the total events fetched so far.
type ProgressFunc func(page int, eventsSoFar int)

// Direction controls whether pagination moves forward or backward through time.
type Direction string

const (
	// DirectionAfter fetches older events (pagination forward in time).
	DirectionAfter Direction = "after"
	// DirectionBefore fetches newer events (pagination backward in time).
	DirectionBefore Direction = "before"
)

// TRClient defines the subset of client.Client methods needed for fetching.
// This interface enables testing with mocks.
type TRClient interface {
	TimelineTransactions(ctx context.Context, after *string) (string, error)
	TimelineDetailV2(ctx context.Context, timelineID string) (string, error)
	Unsubscribe(ctx context.Context, subscriptionID string) error
	Recv() <-chan client.Message
}

// FetchAllEvents fetches all timeline events with their details from the Trade Republic API.
// It paginates through all pages in the given direction, calling progressFn after each page.
// progressFn may be nil if no progress reporting is needed.
// Returns raw combined maps preserving all API fields (including unknown ones).
// Use ParseRawMaps to convert to typed RawEvent objects when needed.
func FetchAllEvents(ctx context.Context, trclient TRClient, direction Direction, cursor *string, progressFn ProgressFunc) ([]map[string]interface{}, error) {
	var allRawMaps []map[string]interface{}
	page := 0

	for {
		page++

		rawItems, nextCursors, err := fetchTimelinePage(ctx, trclient, cursor)
		if err != nil {
			return nil, err
		}

		details, err := fetchDetailsForItems(ctx, trclient, rawItems)
		if err != nil {
			return nil, err
		}

		rawMaps := assembleCombinedMaps(rawItems, details)
		allRawMaps = append(allRawMaps, rawMaps...)

		if progressFn != nil {
			progressFn(page, len(allRawMaps))
		}

		nextCursor := nextCursors.forDirection(direction)

		if nextCursor == nil || len(rawItems) == 0 {
			break
		}

		cursor = nextCursor
		log.Info("Fetching next page", zap.Int("totalSoFar", len(allRawMaps)), zap.Stringp("nextCursor", nextCursor))
	}

	return allRawMaps, nil
}

// assembleCombinedMaps combines timeline items with their details into raw maps.
// The maps preserve all API fields (including unknown ones) for disk persistence.
func assembleCombinedMaps(rawItems []map[string]interface{}, details map[string]map[string]interface{}) []map[string]interface{} {
	maps := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		id, _ := item["id"].(string)
		combined := map[string]interface{}{
			"timelineEvent": item,
			"details":       details[id],
		}
		maps = append(maps, combined)
	}
	return maps
}

// FormatCursorForDirection takes an "after" cursor from a page response
// and converts it to the specified direction for saving.
func FormatCursorForDirection(afterCursor *string) (*string, error) {
	if afterCursor == nil {
		return nil, nil
	}

	return ChangeCursor(afterCursor, DirectionBefore)
}

// ItemID extracts the ID from a raw timeline item map.
func ItemID(item map[string]interface{}) string {
	id, _ := item["id"].(string)
	return id
}

// ItemTimestamp extracts the timestamp string from a raw timeline item map.
func ItemTimestamp(item map[string]interface{}) string {
	ts, _ := item["timestamp"].(string)
	return ts
}

// ItemStatus extracts the status string from a raw timeline item map.
func ItemStatus(item map[string]interface{}) string {
	status, _ := item["status"].(string)
	return strings.ToUpper(status)
}

// ItemTimestampDate extracts the first 10 characters of the timestamp (YYYY-MM-DD).
func ItemTimestampDate(item map[string]interface{}) string {
	ts := ItemTimestamp(item)
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ""
}

// ParseTimestamp parses a Trade Republic timestamp string into a time.Time.
func ParseTimestamp(timestampStr string) (time.Time, error) {
	return time.Parse(types.TimestampLayout, timestampStr)
}

// FormatSubscriptionID converts an integer subscription counter to a string.
func FormatSubscriptionID(id int) string {
	return strconv.Itoa(id)
}
