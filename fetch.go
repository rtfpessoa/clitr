package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	phoneFlagKeyShort = "p"
	phoneFlagKeyLong  = "phone"

	saveCredentialsFlagKeyShort = "s"
	saveCredentialsFlagKeyLong  = "save-credentials"

	incrementalFlagKeyShort = "i"
	incrementalFlagKeyLong  = "incremental"

	resetFlagKeyShort = "r"
	resetFlagKeyLong  = "reset"

	resetCredentialsFlagKeyLong = "reset-credentials"
	resetDataFlagKeyLong        = "reset-data"
)

// rawEventFile holds unvalidated API response data for saving to disk.
// It produces the same JSON schema as types.RawEvent but accepts any map content,
// deferring strict validation to export time.
type rawEventFile struct {
	TimelineEvent interface{} `json:"timelineEvent"`
	Details       interface{} `json:"details"`
	PageCursor    *string     `json:"pageCursor"`
}

type fetchConfig struct {
	*rootConfig

	phoneNo          string
	saveCredentials  bool
	incremental      bool
	reset            bool
	resetCredentials bool
	resetData        bool
}

func NewFetchCmd(rootConfig *rootConfig) *cobra.Command {
	fetchConfig := &fetchConfig{
		rootConfig: rootConfig,
	}

	// Fetch command - downloads data from Trade Republic
	fetchCmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch transactions from Trade Republic",
		Long:  "Authenticate with Trade Republic and download all transaction data to JSON files",
	}

	fetchCmd.PersistentFlags().StringVarP(&fetchConfig.phoneNo, phoneFlagKeyLong, phoneFlagKeyShort, "", "Phone number in international format (e.g., +4912345678)")
	fetchCmd.PersistentFlags().BoolVarP(&fetchConfig.saveCredentials, saveCredentialsFlagKeyLong, saveCredentialsFlagKeyShort, false, "Save session cookies for future use")
	fetchCmd.PersistentFlags().BoolVarP(&fetchConfig.incremental, incrementalFlagKeyLong, incrementalFlagKeyShort, false, "Fetch only new transactions and update pending ones")
	fetchCmd.PersistentFlags().BoolVarP(&fetchConfig.reset, resetFlagKeyLong, resetFlagKeyShort, false, "Reset all: clear credentials and delete transaction data")
	fetchCmd.PersistentFlags().BoolVar(&fetchConfig.resetCredentials, resetCredentialsFlagKeyLong, false, "Clear saved credentials from system keyring")
	fetchCmd.PersistentFlags().BoolVar(&fetchConfig.resetData, resetDataFlagKeyLong, false, "Delete transaction data from disk")

	fetchCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runFetch(cmd.Context(), fetchConfig)
	}

	return fetchCmd
}

func runFetch(ctx context.Context, config *fetchConfig) error {
	// Handle reset flags
	resetData := config.resetData || config.reset
	resetCredentials := config.resetCredentials || config.reset

	if err := handleResetData(resetData, config.dataDir); err != nil {
		return err
	}

	if err := handleResetCredentials(resetCredentials, config.phoneNo); err != nil {
		return err
	}

	trclient, err := client.NewClient(config.phoneNo, config.dataDir, config.saveCredentials)
	if err != nil {
		return err
	}
	defer func(client *client.Client) {
		err := client.Close()
		if err != nil {
			log.Error("Failed to close client", zap.Error(err))
		}
	}(trclient)
	err = trclient.AuthenticateClient()
	if err != nil {
		return err
	}

	rawEvents, err := fetchEvents(ctx, trclient, config.eventsDir, config.incremental)
	if err != nil {
		return err
	}

	if len(rawEvents) == 0 {
		log.Info("No new transactions found")
		return nil
	}

	return saveRawEvents(rawEvents, config.eventsDir)
}

func handleResetData(reset bool, dir string) error {
	if !reset {
		return nil
	}

	log.Info("Resetting data directory", zap.String("dir", dir))
	if err := os.RemoveAll(dir); err != nil {
		log.Error("failed to remove data directory", zap.Error(err))
		return err
	}

	return nil
}

func handleResetCredentials(reset bool, phoneNo string) error {
	if !reset {
		return nil
	}

	log.Info("Resetting credentials from keyring", zap.String("phone", phoneNo))
	// Create a temporary client just to reset credentials
	tempClient, err := client.NewClient(phoneNo, "", false)
	if err != nil {
		return fmt.Errorf("failed to create client for credential reset: %w", err)
	}

	if err := tempClient.ResetCookies(); err != nil {
		log.Error("failed to reset credentials", zap.Error(err))
		return err
	}

	return nil
}

func fetchEvents(ctx context.Context, trclient *client.Client, eventsDir string, incremental bool) ([]*rawEventFile, error) {
	log.Info("Fetching transaction history")

	var cursor *string
	direction := FetchDirectionAfter

	if incremental {
		meta, err := loadMetadata(eventsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load metadata: %w", err)
		}
		if meta == nil {
			log.Info("No previous events found, going to do a full fetch")
		} else {
			if meta.PendingCount > 0 {
				log.Info("Found pending transactions from last fetch", zap.Int("count", meta.PendingCount))
				var oldestPendingItem *pendingItem
				for _, item := range meta.PendingItems {
					if oldestPendingItem == nil || item.Timestamp.Before(oldestPendingItem.Timestamp) {
						oldestPendingItem = &item
					}
				}

				if oldestPendingItem == nil {
					return nil, fmt.Errorf("failed to find pending transactions from last fetch")
				}

				cursor = oldestPendingItem.PageCursor
			} else if meta.LatestPageCursor != nil {
				cursor = meta.LatestPageCursor
			}

			direction = FetchDirectionBefore
			log.Info("Processing incremental fetch", zap.Any("fromItem", cursor))
		}
	}

	rawEvents, err := fetchRawEvents(ctx, trclient, direction, cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}
	log.Info("Retrieved transactions", zap.Int("count", len(rawEvents)))

	return rawEvents, nil
}

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

type FetchDirection string

const (
	// Oldest events
	FetchDirectionAfter FetchDirection = "after"
	// Newest events
	FetchDirectionBefore FetchDirection = "before"
)

func fetchRawEvents(ctx context.Context, trclient *client.Client, fetchDirection FetchDirection, cursor *string) ([]*rawEventFile, error) {
	var allRawEvents []*rawEventFile

	for {
		rawItems, nextPageCursor, err := fetchTimelinePage(ctx, trclient, cursor)
		if err != nil {
			return nil, err
		}

		details, err := fetchDetailsForItems(ctx, trclient, rawItems)
		if err != nil {
			return nil, err
		}

		var eventCursor *string
		if nextPageCursor.After != nil {
			eventCursor = changeCursor(nextPageCursor.After, FetchDirectionBefore)
		} else {
			eventCursor = cursor
		}

		rawEvents := assembleRawEvents(rawItems, details, eventCursor)
		allRawEvents = append(allRawEvents, rawEvents...)

		var nextCursor *string
		switch fetchDirection {
		case FetchDirectionAfter:
			nextCursor = nextPageCursor.After
		case FetchDirectionBefore:
			nextCursor = nextPageCursor.Before
		}

		if nextCursor == nil || len(rawItems) == 0 {
			break
		}

		cursor = nextCursor
		log.Info("Fetching next page", zap.Int("totalSoFar", len(allRawEvents)), zap.Stringp("nextCursor", nextCursor))
	}

	return allRawEvents, nil
}

func fetchTimelinePage(ctx context.Context, trclient *client.Client, after *string) ([]map[string]interface{}, *pageCursor, error) {
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
				rawItems, cursor, err := parseTimelineMessage(msg)
				if err != nil {
					return nil, nil, err
				}

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

type pageCursor struct {
	Before *string `json:"before"`
	After  *string `json:"after"`
}

func parseTimelineMessage(msg client.Message) ([]map[string]interface{}, *pageCursor, error) {
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

	return rawItems, cursor, nil
}

func fetchDetailsForItems(ctx context.Context, trclient *client.Client, rawItems []map[string]interface{}) (map[string]map[string]interface{}, error) {
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

func assembleRawEvents(rawItems []map[string]interface{}, details map[string]map[string]interface{}, pageCursor *string) []*rawEventFile {
	rawEvents := make([]*rawEventFile, 0, len(rawItems))
	for _, item := range rawItems {
		id, _ := item["id"].(string)
		rawEvent := &rawEventFile{
			TimelineEvent: item,
			Details:       details[id],
			PageCursor:    pageCursor,
		}
		rawEvents = append(rawEvents, rawEvent)
	}
	return rawEvents
}

// getKeys extracts all keys from a map for debugging output
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

type cursor struct {
	Keyset struct {
		EventId   string    `json:"eventId"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"keyset"`
	PageDirection FetchDirection `json:"pageDirection"`
}

func changeCursor(cursorStr *string, direction FetchDirection) *string {
	if cursorStr == nil {
		if direction == FetchDirectionBefore {
			log.Error("Cannot change direction to before from empty cursor")
		}
		return nil
	}

	bytes, err := base64.RawStdEncoding.DecodeString(*cursorStr)
	if err != nil {
		log.Error("Failed to decode cursor", zap.Error(err))
		return nil
	}

	parsed := &cursor{}
	err = json.Unmarshal(bytes, parsed)
	if err != nil {
		log.Error("Failed to decode cursor", zap.Error(err))
		return nil
	}

	parsed.PageDirection = direction
	cursorBytes, err := json.Marshal(parsed)
	if err != nil {
		log.Error("Failed to encode cursor", zap.Error(err))
		return nil
	}

	newCursor := base64.RawStdEncoding.EncodeToString(cursorBytes)

	return &newCursor
}
