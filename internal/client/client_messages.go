package client

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

type wsFrame struct {
	subscriptionID string
	code           byte
	payload        string
}

func parseWSFrame(data string) (wsFrame, error) {
	parts := strings.SplitN(data, " ", 2)
	if len(parts) != 2 || len(parts[1]) == 0 {
		return wsFrame{}, fmt.Errorf("failed to parse message: %s", data)
	}
	return wsFrame{subscriptionID: parts[0], code: parts[1][0], payload: parts[1][1:]}, nil
}

// parseMessage parses a WebSocket message.
func (c *Client) parseMessage(data string) (*Message, error) {
	frame, err := parseWSFrame(data)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	sub, ok := c.subscriptions[frame.subscriptionID]
	c.mu.Unlock()
	if !ok && frame.code != 'C' {
		return nil, fmt.Errorf("no active subscription: %s", data)
	}

	var message *Message
	switch frame.code {
	case 'A', 'D':
		message, err = c.parseDataFrame(frame, sub)
	case 'C', 'E':
		message, err = c.parseTerminalFrame(frame, sub)
	}
	return message, err
}

func (c *Client) parseDataFrame(frame wsFrame, sub Subscription) (*Message, error) {
	payload := frame.payload
	if frame.code == 'D' {
		c.mu.Lock()
		previous := c.previousResponses[frame.subscriptionID]
		c.mu.Unlock()
		payload = calculateDelta(previous, payload)
	}
	c.mu.Lock()
	c.previousResponses[frame.subscriptionID] = payload
	c.mu.Unlock()
	decoded, err := decodeMessagePayload(payload)
	if err != nil {
		return nil, err
	}
	return &Message{SubscriptionID: frame.subscriptionID, Subscription: sub, Payload: decoded}, nil
}

func (c *Client) parseTerminalFrame(frame wsFrame, sub Subscription) (*Message, error) {
	c.mu.Lock()
	delete(c.subscriptions, frame.subscriptionID)
	delete(c.previousResponses, frame.subscriptionID)
	c.mu.Unlock()
	if frame.code == 'C' {
		return nil, nil
	}
	decoded, err := decodeMessagePayload(frame.payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		SubscriptionID: frame.subscriptionID,
		Subscription:   sub,
		Payload:        decoded,
		Error:          fmt.Errorf("subscription error: %s", frame.payload),
	}, nil
}

func decodeMessagePayload(raw string) (map[string]interface{}, error) {
	if raw == "" {
		return nil, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// calculateDelta applies the WebSocket delta format to the previous payload.
func calculateDelta(previous, delta string) string {
	state := deltaState{previous: previous}
	for _, part := range strings.Split(delta, "\t") {
		if part == "" {
			continue
		}
		if err := state.apply(part); err != nil {
			log.Error("failed to apply delta", zap.Error(err))
			return ""
		}
	}
	return state.result.String()
}

type deltaState struct {
	previous string
	offset   int
	result   strings.Builder
}

func (s *deltaState) apply(part string) error {
	switch part[0] {
	case '+':
		decoded, err := url.QueryUnescape(strings.TrimSpace(part))
		if err != nil {
			return fmt.Errorf("decode inserted text: %w", err)
		}
		s.result.WriteString(decoded)
	case '-', '=':
		if len(part) < 2 {
			return nil
		}
		length, err := strconv.Atoi(part[1:])
		if err != nil || length < 0 {
			return fmt.Errorf("invalid delta length %q", part[1:])
		}
		if part[0] == '=' && s.offset+length <= len(s.previous) {
			s.result.WriteString(s.previous[s.offset : s.offset+length])
		}
		s.offset += length
	}
	return nil
}
