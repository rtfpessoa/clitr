package fetch

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"testing"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTRClient implements TRClient for testing.
type mockTRClient struct {
	messages        []client.Message
	msgIndex        int
	msgCh           chan client.Message
	subscriptions   []string
	unsubscribed    []string
	timelineCallArg *string
	detailCallIDs   []string
	subIDCounter    int
}

func newMockClient() *mockTRClient {
	return &mockTRClient{
		msgCh: make(chan client.Message, 100),
	}
}

func (m *mockTRClient) TimelineTransactions(_ context.Context, after *string) (string, error) {
	m.timelineCallArg = after
	m.subIDCounter++
	subID := fmt.Sprintf("sub-%d", m.subIDCounter)
	m.subscriptions = append(m.subscriptions, subID)
	return subID, nil
}

func (m *mockTRClient) TimelineDetailV2(_ context.Context, timelineID string) (string, error) {
	m.detailCallIDs = append(m.detailCallIDs, timelineID)
	m.subIDCounter++
	subID := fmt.Sprintf("sub-%d", m.subIDCounter)
	m.subscriptions = append(m.subscriptions, subID)
	return subID, nil
}

func (m *mockTRClient) Unsubscribe(_ context.Context, subscriptionID string) error {
	m.unsubscribed = append(m.unsubscribed, subscriptionID)
	return nil
}

func (m *mockTRClient) Recv() <-chan client.Message {
	return m.msgCh
}

func (m *mockTRClient) sendMessage(msg client.Message) {
	m.msgCh <- msg
}

// --- parseTimelineMessage tests (moved from fetch_test.go) ---

func TestParseTimelineMessage_ValidItems(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"id":        "event-1",
					"timestamp": "2024-01-15T10:30:00.000+0100",
					"title":     "Apple Inc.",
					"status":    "EXECUTED",
				},
			},
		},
	}

	items, cursor := parseTimelineMessage(msg)
	require.Len(t, items, 1)

	assert.Equal(t, "event-1", items[0]["id"])
	assert.Equal(t, "Apple Inc.", items[0]["title"])
	assert.NotNil(t, cursor)
}

func TestParseTimelineMessage_WithCursors(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{},
			"cursors": map[string]interface{}{
				"before": "cursor-before",
				"after":  "cursor-after",
			},
		},
	}

	_, cursor := parseTimelineMessage(msg)
	require.NotNil(t, cursor)

	assert.Equal(t, "cursor-before", *cursor.Before)
	assert.Equal(t, "cursor-after", *cursor.After)
}

func TestParseTimelineMessage_PreservesUnknownFields(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"id":           "event-1",
					"timestamp":    "2024-01-15T10:30:00.000+0100",
					"title":        "Apple Inc.",
					"status":       "EXECUTED",
					"unknownField": "preserved",
				},
			},
		},
	}

	items, _ := parseTimelineMessage(msg)
	require.Len(t, items, 1)

	// Unknown fields are preserved in the raw map
	assert.Equal(t, "preserved", items[0]["unknownField"])
}

func TestParseTimelineMessage_EmptyPayload(t *testing.T) {
	msg := client.Message{
		SubscriptionID: "1",
		Payload:        map[string]interface{}{},
	}

	items, cursor := parseTimelineMessage(msg)
	assert.Empty(t, items)
	assert.NotNil(t, cursor)
	assert.Nil(t, cursor.After)
	assert.Nil(t, cursor.Before)
}

// --- ChangeCursor tests (moved from fetch_test.go) ---

func TestChangeCursor_DirectionChange(t *testing.T) {
	cursorJSON := `{"keyset":{"eventId":"test-123","timestamp":"2024-01-15T10:30:00Z"},"pageDirection":"after"}`
	encoded := base64.RawStdEncoding.EncodeToString([]byte(cursorJSON))

	result, err := ChangeCursor(&encoded, DirectionBefore)
	require.NoError(t, err)
	require.NotNil(t, result)

	decoded, err := base64.RawStdEncoding.DecodeString(*result)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), `"pageDirection": "before"`)
}

func TestChangeCursor_NilInput(t *testing.T) {
	result, err := ChangeCursor(nil, DirectionAfter)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestChangeCursor_NilInputBeforeDirection(t *testing.T) {
	_, err := ChangeCursor(nil, DirectionBefore)
	require.Error(t, err)
}

func TestChangeCursor_InvalidBase64(t *testing.T) {
	invalid := "not-valid-base64!!!"
	_, err := ChangeCursor(&invalid, DirectionAfter)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode cursor")
}

func TestChangeCursor_InvalidJSON(t *testing.T) {
	encoded := base64.RawStdEncoding.EncodeToString([]byte("not json"))
	_, err := ChangeCursor(&encoded, DirectionAfter)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal cursor")
}

// --- FormatCursorForDirection tests ---

func TestFormatCursorForDirection_NilInput(t *testing.T) {
	result, err := FormatCursorForDirection(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestFormatCursorForDirection_ChangesToBefore(t *testing.T) {
	cursorJSON := `{"keyset":{"eventId":"test-123","timestamp":"2024-01-15T10:30:00Z"},"pageDirection":"after"}`
	encoded := base64.RawStdEncoding.EncodeToString([]byte(cursorJSON))

	result, err := FormatCursorForDirection(&encoded)
	require.NoError(t, err)
	require.NotNil(t, result)

	decoded, err := base64.RawStdEncoding.DecodeString(*result)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), `"pageDirection": "before"`)
}

// --- Helper function tests ---

func TestItemID(t *testing.T) {
	item := map[string]interface{}{"id": "test-id"}
	assert.Equal(t, "test-id", ItemID(item))
}

func TestItemID_Missing(t *testing.T) {
	item := map[string]interface{}{"title": "no id"}
	assert.Equal(t, "", ItemID(item))
}

func TestItemTimestamp(t *testing.T) {
	item := map[string]interface{}{"timestamp": "2024-01-15T10:30:00.000+0100"}
	assert.Equal(t, "2024-01-15T10:30:00.000+0100", ItemTimestamp(item))
}

func TestItemStatus(t *testing.T) {
	item := map[string]interface{}{"status": "executed"}
	assert.Equal(t, "EXECUTED", ItemStatus(item))
}

func TestItemTimestampDate(t *testing.T) {
	item := map[string]interface{}{"timestamp": "2024-01-15T10:30:00.000+0100"}
	assert.Equal(t, "2024-01-15", ItemTimestampDate(item))
}

func TestItemTimestampDate_Short(t *testing.T) {
	item := map[string]interface{}{"timestamp": "short"}
	assert.Equal(t, "", ItemTimestampDate(item))
}

func TestFormatSubscriptionID(t *testing.T) {
	assert.Equal(t, "42", FormatSubscriptionID(42))
}

// --- assembleRawEvents tests ---

func TestAssembleRawEvents_Basic(t *testing.T) {
	items := []map[string]interface{}{
		{"id": "e1", "timestamp": "2024-01-15T10:30:00.000+0100", "title": "Event 1", "status": "EXECUTED", "eventType": "order_executed"},
		{"id": "e2", "timestamp": "2024-01-16T10:30:00.000+0100", "title": "Event 2", "status": "EXECUTED", "eventType": "incoming_transfer"},
	}
	details := map[string]map[string]interface{}{
		"e1": {"id": "e1", "sections": []interface{}{}},
		"e2": {"id": "e2", "sections": []interface{}{}},
	}

	result, err := assembleRawEvents(items, details)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "e1", result[0].TimelineEvent.ID)
	assert.Equal(t, "e2", result[1].TimelineEvent.ID)
}

func TestAssembleRawEvents_MissingDetail(t *testing.T) {
	items := []map[string]interface{}{
		{"id": "e1", "timestamp": "2024-01-15T10:30:00.000+0100", "title": "Event 1", "status": "EXECUTED", "eventType": "order_executed"},
	}
	details := map[string]map[string]interface{}{
		// No detail for e1
	}

	result, err := assembleRawEvents(items, details)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "e1", result[0].TimelineEvent.ID)
}

func TestAssembleRawEvents_Empty(t *testing.T) {
	result, err := assembleRawEvents(nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// --- FetchAllEvents integration tests ---

func TestFetchAllEvents_SinglePage(t *testing.T) {
	mock := newMockClient()
	ctx := context.Background()

	var progressCalls []struct{ page, events int }

	// Run FetchAllEvents in a goroutine since it blocks on Recv()
	resultCh := make(chan struct {
		events []*typesRawEvent
		err    error
	}, 1)

	type result struct {
		events interface{}
		err    error
	}

	go func() {
		events, err := FetchAllEvents(ctx, mock, DirectionAfter, nil, func(page, eventsSoFar int) {
			progressCalls = append(progressCalls, struct{ page, events int }{page, eventsSoFar})
		})
		resultCh <- struct {
			events []*typesRawEvent
			err    error
		}{events: nil, err: err}
		_ = events
	}()

	// Wait for TimelineTransactions to be called, then send timeline response
	// The mock sends on the channel immediately, so we send the timeline message
	// after a brief moment to let FetchAllEvents start reading
	timelinePayload := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"id":        "event-1",
				"timestamp": "2024-01-15T10:30:00.000+0100",
				"title":     "Apple Inc.",
				"status":    "EXECUTED",
				"eventType": "order_executed",
			},
		},
		"cursors": map[string]interface{}{},
	}

	// Send timeline page response (subscription ID will be "sub-1")
	mock.sendMessage(client.Message{
		SubscriptionID: "sub-1",
		Payload:        timelinePayload,
	})

	// Send detail response (subscription ID will be "sub-2")
	mock.sendMessage(client.Message{
		SubscriptionID: "sub-2",
		Payload: map[string]interface{}{
			"id":       "event-1",
			"sections": []interface{}{},
		},
	})

	res := <-resultCh
	require.NoError(t, res.err)
}

func TestFetchAllEvents_ErrorFromWebSocket(t *testing.T) {
	mock := newMockClient()
	ctx := context.Background()

	go func() {
		// Send an error message
		mock.sendMessage(client.Message{
			SubscriptionID: "sub-1",
			Error:          fmt.Errorf("websocket connection lost"),
		})
	}()

	_, err := FetchAllEvents(ctx, mock, DirectionAfter, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "websocket connection lost")
}

func TestFetchAllEvents_ContextCancellation(t *testing.T) {
	mock := newMockClient()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Cancel context after a brief moment
		cancel()
	}()

	_, err := FetchAllEvents(ctx, mock, DirectionAfter, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchAllEvents_NilProgressFn(t *testing.T) {
	mock := newMockClient()
	ctx := context.Background()

	go func() {
		// Send a timeline with no items and no cursors (empty result)
		mock.sendMessage(client.Message{
			SubscriptionID: "sub-1",
			Payload: map[string]interface{}{
				"items":   []interface{}{},
				"cursors": map[string]interface{}{},
			},
		})
	}()

	events, err := FetchAllEvents(ctx, mock, DirectionAfter, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestParseTimestamp(t *testing.T) {
	ts, err := ParseTimestamp("2024-01-15T10:30:00.000+0100")
	require.NoError(t, err)
	assert.Equal(t, 2024, ts.Year())
	assert.Equal(t, 1, int(ts.Month()))
	assert.Equal(t, 15, ts.Day())
}

func TestParseTimestamp_Invalid(t *testing.T) {
	_, err := ParseTimestamp("not-a-timestamp")
	require.Error(t, err)
}

// typesRawEvent is imported as types.RawEvent but we need the alias for the channel
type typesRawEvent = struct {
	TimelineEvent interface{}
	Details       interface{}
	PageCursor    *string
}

// --- getKeys test ---

func TestGetKeys(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	keys := getKeys(m)
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "a")
	assert.Contains(t, keys, "b")
	assert.Contains(t, keys, "c")
}

// --- Direction constants ---

func TestDirectionConstants(t *testing.T) {
	assert.Equal(t, Direction("after"), DirectionAfter)
	assert.Equal(t, Direction("before"), DirectionBefore)
}

// --- Cursor round-trip with actual JSON ---

func TestChangeCursor_RoundTrip(t *testing.T) {
	// Create a cursor with "after" direction
	original := cursorPayload{}
	original.Keyset.EventId = "evt-456"
	original.PageDirection = DirectionAfter

	jsonBytes, err := stdjson.Marshal(original)
	require.NoError(t, err)

	encoded := base64.RawStdEncoding.EncodeToString(jsonBytes)

	// Change to "before"
	result, err := ChangeCursor(&encoded, DirectionBefore)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Decode and verify
	decoded, err := base64.RawStdEncoding.DecodeString(*result)
	require.NoError(t, err)

	var parsed cursorPayload
	require.NoError(t, stdjson.Unmarshal(decoded, &parsed))

	assert.Equal(t, "evt-456", parsed.Keyset.EventId)
	assert.Equal(t, DirectionBefore, parsed.PageDirection)
}
