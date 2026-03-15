package export

import (
	"testing"

	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRawEvents_ValidEvents(t *testing.T) {
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "test-1",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Apple Inc.",
				Status:    "EXECUTED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
				Amount:    &types.Amount{Value: -100.0, Currency: "EUR"},
			},
			Details: types.TimelineDetails{
				ID:       "test-1",
				Sections: []types.Section{},
			},
		},
	}

	events, err := ParseRawEvents(rawEvents)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, "test-1", events[0].ID)
	assert.Equal(t, types.EventTypeTradeInvoice, events[0].EventType)
}

func TestParseRawEvents_SkipCanceled(t *testing.T) {
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "canceled-1",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Canceled Order",
				Status:    "CANCELED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
			},
			Details: types.TimelineDetails{ID: "canceled-1"},
		},
	}

	events, err := ParseRawEvents(rawEvents)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestParseRawEvents_Empty(t *testing.T) {
	events, err := ParseRawEvents([]*types.RawEvent{})
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestParseRawEvents_MultipleEvents(t *testing.T) {
	rawEvents := []*types.RawEvent{
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "buy-1",
				Timestamp: "2024-01-15T10:30:00.000+0100",
				Title:     "Apple Inc.",
				Status:    "EXECUTED",
				EventType: "order_executed",
				Subtitle:  "Buy Order",
				Amount:    &types.Amount{Value: -100.0, Currency: "EUR"},
			},
			Details: types.TimelineDetails{
				ID:       "buy-1",
				Sections: []types.Section{},
			},
		},
		{
			TimelineEvent: types.TimelineEvent{
				ID:        "deposit-1",
				Timestamp: "2024-01-16T10:30:00.000+0100",
				Title:     "Deposit",
				Status:    "EXECUTED",
				EventType: "incoming_transfer",
				Subtitle:  "",
				Amount:    &types.Amount{Value: 500.0, Currency: "EUR"},
			},
			Details: types.TimelineDetails{
				ID:       "deposit-1",
				Sections: []types.Section{},
			},
		},
	}

	events, err := ParseRawEvents(rawEvents)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, "buy-1", events[0].ID)
	assert.Equal(t, "deposit-1", events[1].ID)
}
