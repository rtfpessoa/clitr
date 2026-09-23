package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/fetch"
	"github.com/rtfpessoa/clitr/internal/log"
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
	start := fetchStart{direction: fetch.DirectionAfter}
	if incremental {
		var err error
		start, err = incrementalFetchStart(eventsDir)
		if err != nil {
			return nil, err
		}
	}

	rawEvents, err := fetchRawEvents(ctx, trclient, start.direction, start.cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}
	log.Info("Retrieved transactions", zap.Int("count", len(rawEvents)))

	return rawEvents, nil
}

type fetchStart struct {
	direction fetch.Direction
	cursor    *string
}

func incrementalFetchStart(eventsDir string) (fetchStart, error) {
	meta, err := loadMetadata(eventsDir)
	if err != nil {
		return fetchStart{}, fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta == nil {
		log.Info("No previous events found, going to do a full fetch")
		return fetchStart{direction: fetch.DirectionAfter}, nil
	}
	start := fetchStart{direction: fetch.DirectionBefore, cursor: meta.LatestPageCursor}
	if meta.PendingCount > 0 {
		log.Info("Found pending transactions from last fetch", zap.Int("count", meta.PendingCount))
		start.cursor, err = oldestPendingCursor(meta.PendingItems)
		if err != nil {
			return fetchStart{}, err
		}
	}
	log.Info("Processing incremental fetch", zap.Any("fromItem", start.cursor))
	return start, nil
}

func oldestPendingCursor(items []pendingItem) (*string, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("failed to find pending transactions from last fetch")
	}
	oldest := &items[0]
	for i := 1; i < len(items); i++ {
		if items[i].Timestamp.Before(oldest.Timestamp) {
			oldest = &items[i]
		}
	}
	return oldest.PageCursor, nil
}

// fetchRawEvents delegates to fetch.FetchAllEvents and converts the raw maps
// to rawEventFile for disk serialization, preserving all API fields.
func fetchRawEvents(ctx context.Context, trclient *client.Client, direction fetch.Direction, cursor *string) ([]*rawEventFile, error) {
	rawMaps, err := fetch.FetchAllEvents(ctx, trclient, direction, cursor, func(page int, eventsSoFar int) {
		log.Info("Fetched page", zap.Int("page", page), zap.Int("eventsSoFar", eventsSoFar))
	})
	if err != nil {
		return nil, err
	}

	return convertMapsToRawEventFiles(rawMaps, cursor), nil
}

// convertMapsToRawEventFiles converts raw combined maps to rawEventFile objects
// for disk persistence. The maps contain all API fields (including unknown ones),
// which are preserved because rawEventFile uses interface{} fields.
func convertMapsToRawEventFiles(rawMaps []map[string]interface{}, fallbackCursor *string) []*rawEventFile {
	result := make([]*rawEventFile, 0, len(rawMaps))
	for _, m := range rawMaps {
		raw := &rawEventFile{
			TimelineEvent: m["timelineEvent"],
			Details:       m["details"],
			PageCursor:    fallbackCursor,
		}
		result = append(result, raw)
	}
	return result
}
