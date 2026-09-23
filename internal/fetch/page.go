package fetch

import (
	"context"
	"fmt"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// fetchTimelinePage fetches one page through a WebSocket subscription.
func fetchTimelinePage(ctx context.Context, trclient TRClient, after *string) ([]map[string]interface{}, *pageCursor, error) {
	subID, err := trclient.TimelineTransactions(ctx, after)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to subscribe to timeline transactions: %w", err)
	}
	response := awaitTimelinePage(ctx, trclient.Recv(), subID)
	if response.err == nil {
		response.err = trclient.Unsubscribe(ctx, subID)
	}
	if response.err != nil {
		return nil, nil, response.err
	}
	return response.items, response.cursor, nil
}

type timelinePageResponse struct {
	items  []map[string]interface{}
	cursor *pageCursor
	err    error
}

func awaitTimelinePage(ctx context.Context, responses <-chan client.Message, subID string) timelinePageResponse {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	var result timelinePageResponse
wait:
	for {
		select {
		case msg, ok := <-responses:
			if !ok {
				result.err = fmt.Errorf("timeline response channel closed")
				break wait
			}
			if msg.Error != nil {
				result.err = msg.Error
				break wait
			}
			if msg.SubscriptionID == subID {
				result.items, result.cursor = parseTimelineMessage(msg)
				break wait
			}
			log.Warn("Received message for unknown subscription", zap.Any("msg", msg))
		case <-ctx.Done():
			result.err = ctx.Err()
			break wait
		case <-timer.C:
			result.err = fmt.Errorf("timeout waiting for timeline response")
			break wait
		}
	}
	return result
}

// parseTimelineMessage extracts timeline items and cursors from a WebSocket message.
func parseTimelineMessage(msg client.Message) ([]map[string]interface{}, *pageCursor) {
	log.Debug("Received timeline message",
		zap.String("subscriptionID", msg.SubscriptionID),
		zap.Strings("payloadKeys", getKeys(msg.Payload)))

	return timelineItems(msg.Payload), timelineCursors(msg.Payload)
}

func timelineItems(payload map[string]interface{}) []map[string]interface{} {
	var rawItems []map[string]interface{}
	if items, ok := payload["items"].([]interface{}); ok {
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
	return rawItems
}

func timelineCursors(payload map[string]interface{}) *pageCursor {
	cursor := &pageCursor{}
	if cursors, ok := payload["cursors"].(map[string]interface{}); ok {
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

	return cursor
}

// getKeys extracts all keys from a map for debugging output.
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
