package export

import (
	"fmt"

	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"go.uber.org/zap"
)

// ParseRawEvents converts raw API events into typed Event objects.
// Skips canceled events and events that cannot be parsed.
func ParseRawEvents(rawEvents []*types.RawEvent) ([]*types.Event, error) {
	log.Info("Parsing transactions")
	events := make([]*types.Event, 0, len(rawEvents))

	for _, rawEvent := range rawEvents {
		event, err := types.ParseEvent(rawEvent.TimelineEvent, rawEvent.Details)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}
		if event != nil {
			events = append(events, event)
		}
	}

	log.Info("Parsed transactions", zap.Int("count", len(events)))
	return events, nil
}
