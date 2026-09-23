package fetch

import (
	"context"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

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
