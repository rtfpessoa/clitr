// Package fetch provides the core transaction fetching pipeline.
// It fetches timeline events and their details from the Trade Republic API
// via WebSocket subscriptions, returning typed RawEvent objects.
package fetch

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"go.uber.org/zap"
)

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

// pageCursor holds pagination cursor values from the API response.
type pageCursor struct {
	Before *string
	After  *string
}

// cursorPayload is the decoded structure of a base64-encoded cursor string.
type cursorPayload struct {
	Keyset struct {
		EventId   string    `json:"eventId"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"keyset"`
	PageDirection Direction `json:"pageDirection"`
}

// FetchAllEvents fetches all timeline events with their details from the Trade Republic API.
// It paginates through all pages in the given direction, calling progressFn after each page.
// progressFn may be nil if no progress reporting is needed.
// Returns typed RawEvent objects suitable for parsing into Event objects.
func FetchAllEvents(ctx context.Context, trclient TRClient, direction Direction, cursor *string, progressFn ProgressFunc) ([]*types.RawEvent, error) {
	var allRawEvents []*types.RawEvent
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

		rawEvents, err := assembleRawEvents(rawItems, details)
		if err != nil {
			return nil, fmt.Errorf("failed to assemble raw events: %w", err)
		}
		allRawEvents = append(allRawEvents, rawEvents...)

		if progressFn != nil {
			progressFn(page, len(allRawEvents))
		}

		var nextCursor *string
		switch direction {
		case DirectionAfter:
			nextCursor = nextCursors.After
		case DirectionBefore:
			nextCursor = nextCursors.Before
		}

		if nextCursor == nil || len(rawItems) == 0 {
			break
		}

		cursor = nextCursor
		log.Info("Fetching next page", zap.Int("totalSoFar", len(allRawEvents)), zap.Stringp("nextCursor", nextCursor))
	}

	return allRawEvents, nil
}

// ChangeCursor decodes a base64-encoded cursor string,
// changes its direction, and re-encodes it.
func ChangeCursor(cursorStr *string, direction Direction) (*string, error) {
	if cursorStr == nil {
		if direction == DirectionBefore {
			return nil, fmt.Errorf("cannot change direction to before from empty cursor")
		}
		return nil, nil
	}

	bytes, err := base64.RawStdEncoding.DecodeString(*cursorStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	parsed := &cursorPayload{}
	// Use stdlib json for cursor parsing — no unknown field restriction needed
	err = stdjson.Unmarshal(bytes, parsed)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	parsed.PageDirection = direction
	cursorBytes, err := stdjson.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cursor: %w", err)
	}

	newCursor := base64.RawStdEncoding.EncodeToString(cursorBytes)
	return &newCursor, nil
}

// fetchTimelinePage fetches a single page of timeline events via WebSocket subscription.
func fetchTimelinePage(ctx context.Context, trclient TRClient, after *string) ([]map[string]interface{}, *pageCursor, error) {
	subID, err := trclient.TimelineTransactions(ctx, after)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to subscribe to timeline transactions: %w", err)
	}

	for {
		select {
		case msg := <-trclient.Recv():
			if msg.Error != nil {
				return nil, nil, msg.Error
			}

			if msg.SubscriptionID == subID {
				rawItems, cursor := parseTimelineMessage(msg)

				err = trclient.Unsubscribe(ctx, subID)
				if err != nil {
					return nil, nil, err
				}

				return rawItems, cursor, nil
			}

			log.Warn("Received message for unknown subscription", zap.Any("msg", msg))

		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(30 * time.Second):
			return nil, nil, fmt.Errorf("timeout waiting for timeline response")
		}
	}
}

// parseTimelineMessage extracts timeline items and cursors from a WebSocket message.
func parseTimelineMessage(msg client.Message) ([]map[string]interface{}, *pageCursor) {
	log.Debug("Received timeline message",
		zap.String("subscriptionID", msg.SubscriptionID),
		zap.Strings("payloadKeys", getKeys(msg.Payload)))

	var rawItems []map[string]interface{}
	cursor := &pageCursor{}

	if items, ok := msg.Payload["items"].([]interface{}); ok {
		log.Debug("Found timeline items", zap.Int("count", len(items)))
		for i, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				log.Warn("Timeline item is not a map", zap.Int("index", i))
				continue
			}

			if i == 0 {
				log.Debug("First timeline item", zap.Strings("keys", getKeys(itemMap)))
				if action, ok := itemMap["action"].(map[string]interface{}); ok {
					log.Debug("First item action", zap.Any("action", action))
				}
			}

			rawItems = append(rawItems, itemMap)
		}
	} else {
		log.Warn("No 'items' field in payload")
	}

	if cursors, ok := msg.Payload["cursors"].(map[string]interface{}); ok {
		if afterVal, ok := cursors["after"].(string); ok && afterVal != "" {
			cursor.After = &afterVal
			log.Debug("Next page cursor", zap.String("cursor", *cursor.After))
		}

		if beforeVal, ok := cursors["before"].(string); ok && beforeVal != "" {
			cursor.Before = &beforeVal
			log.Debug("Previous page cursor", zap.String("cursor", *cursor.Before))
		}
	} else {
		log.Debug("No cursors in response")
	}

	return rawItems, cursor
}

// fetchDetailsForItems fetches detail information for each timeline item.
func fetchDetailsForItems(ctx context.Context, trclient TRClient, rawItems []map[string]interface{}) (map[string]map[string]interface{}, error) {
	detailSubIDs := make(map[string]string, len(rawItems))
	for _, item := range rawItems {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		detailSubID, err := trclient.TimelineDetailV2(ctx, id)
		if err != nil {
			log.Warn("Failed to get timeline detail", zap.String("eventID", id), zap.Error(err))
			continue
		}
		detailSubIDs[detailSubID] = id
	}

	totalDetails := len(detailSubIDs)
	if totalDetails == 0 {
		return nil, nil
	}

	details := make(map[string]map[string]interface{}, totalDetails)
	detailsReceived := 0

	timeoutDuration := 30 * time.Second
	timer := time.NewTimer(timeoutDuration)
	defer timer.Stop()

detailLoop:
	for detailsReceived < totalDetails {
		select {
		case msg, ok := <-trclient.Recv():
			if !ok {
				break detailLoop
			}

			if timelineEventID, ok := detailSubIDs[msg.SubscriptionID]; ok {
				detailsReceived++
				timer.Reset(timeoutDuration)

				if msg.Error != nil {
					log.Warn("Detail fetch error",
						zap.String("eventID", timelineEventID),
						zap.Error(msg.Error))
					continue
				}

				if detailsReceived == 1 {
					log.Debug("First detail received",
						zap.Strings("keys", getKeys(msg.Payload)))
				}

				details[timelineEventID] = msg.Payload

				err := trclient.Unsubscribe(ctx, msg.SubscriptionID)
				if err != nil {
					return nil, err
				}
			}
		case <-ctx.Done():
			log.Warn("Context cancelled while waiting for details",
				zap.Int("received", detailsReceived),
				zap.Int("total", totalDetails))
			break detailLoop
		case <-timer.C:
			log.Warn("Timeout waiting for details",
				zap.Int("received", detailsReceived),
				zap.Int("total", totalDetails))
			break detailLoop
		}
	}

	log.Debug("Collected details",
		zap.Int("collected", len(details)),
		zap.Int("requested", totalDetails))

	return details, nil
}

// assembleRawEvents combines timeline items with their details into typed RawEvent objects.
// Uses stdlib encoding/json for the round-trip to tolerate unknown API fields
// (internal/json uses DisallowUnknownFields which would reject new fields added by TR).
func assembleRawEvents(rawItems []map[string]interface{}, details map[string]map[string]interface{}) ([]*types.RawEvent, error) {
	rawEvents := make([]*types.RawEvent, 0, len(rawItems))

	for _, item := range rawItems {
		id, _ := item["id"].(string)

		// Build a combined map matching the RawEvent JSON structure
		combined := map[string]interface{}{
			"timelineEvent": item,
			"details":       details[id],
		}

		// Round-trip through stdlib JSON to convert untyped maps to typed structs.
		// stdlib json does NOT reject unknown fields, preserving forward compatibility.
		jsonBytes, err := stdjson.Marshal(combined)
		if err != nil {
			log.Warn("Failed to marshal raw event", zap.String("eventID", id), zap.Error(err))
			continue
		}

		var rawEvent types.RawEvent
		if err := stdjson.Unmarshal(jsonBytes, &rawEvent); err != nil {
			log.Warn("Failed to unmarshal raw event", zap.String("eventID", id), zap.Error(err))
			continue
		}

		rawEvents = append(rawEvents, &rawEvent)
	}

	return rawEvents, nil
}

// getKeys extracts all keys from a map for debugging output.
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
