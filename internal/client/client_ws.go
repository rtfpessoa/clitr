package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/types"
)

// setWSHost replaces the WebSocket host in tests.
func (c *Client) setWSHost(host string) {
	c.wsHost = host
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.wsCancel != nil {
		c.wsCancel()
	}

	if c.ws != nil {
		return c.ws.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

// connectWS establishes a WebSocket connection
func (c *Client) connectWS(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws != nil && c.wsContext.Err() == nil {
		return nil // already connected
	}

	headers, err := c.webSocketHeaders()
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(ctx, c.wsHost, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	c.ws = conn
	c.wsContext, c.wsCancel = context.WithCancel(ctx)
	if err := completeWebSocketHandshake(c); err != nil {
		c.wsCancel()
		_ = c.ws.Close(websocket.StatusInternalError, "handshake failed")
		c.ws = nil
		return err
	}
	go c.receiveLoop()
	return nil
}

func (c *Client) webSocketHeaders() (http.Header, error) {
	httpURL, err := url.Parse(cookiesHost)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cookies host: %w", err)
	}
	wsURL, err := url.Parse(c.wsHost)
	if err != nil {
		return nil, fmt.Errorf("failed to parse websocket host: %w", err)
	}

	cookieMap := make(map[string]string)
	addWebSocketCookies(cookieMap, c.httpClient.Jar.Cookies(httpURL))
	addWebSocketCookies(cookieMap, c.httpClient.Jar.Cookies(wsURL))

	var cookieStrs []string
	for name, value := range cookieMap {
		cookieStrs = append(cookieStrs, fmt.Sprintf("%s=%s", name, value))
	}

	headers := http.Header{}
	if len(cookieStrs) > 0 {
		headers.Set("Cookie", strings.Join(cookieStrs, "; "))
	}
	return headers, nil
}

func addWebSocketCookies(values map[string]string, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		if strings.Contains(cookie.Domain, "traderepublic.com") || cookie.Domain == "" {
			values[cookie.Name] = cookie.Value
		}
	}
}

func completeWebSocketHandshake(c *Client) error {
	message, err := webSocketConnectMessage()
	if err != nil {
		return err
	}
	return c.sendWebSocketConnect(message)
}

func webSocketConnectMessage() ([]byte, error) {
	connectMsg := map[string]interface{}{
		"locale":          types.DefaultLanguage,
		"platformId":      "webtrading",
		"platformVersion": "chrome - 142.0.0",
		"clientId":        "app.traderepublic.com",
		"clientVersion":   "11.4.1",
	}
	const webSocketAPIVersion = 33

	msgJSON, err := json.Marshal(connectMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message body: %w", err)
	}
	return []byte(fmt.Sprintf("connect %d %s", webSocketAPIVersion, msgJSON)), nil
}

func (c *Client) sendWebSocketConnect(message []byte) error {
	if err := c.ws.Write(c.wsContext, websocket.MessageText, message); err != nil {
		return fmt.Errorf("failed to send connect message: %w", err)
	}
	msgType, response, err := c.ws.Read(c.wsContext)
	if err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	return validateWebSocketConnectResponse(msgType, response)
}

func validateWebSocketConnectResponse(msgType websocket.MessageType, response []byte) error {
	if msgType != websocket.MessageText {
		return fmt.Errorf("expected text message, got %s", msgType)
	}

	if string(response) != "connected" {
		return fmt.Errorf("connection failed with message type '%s': %s", msgType, string(response))
	}
	return nil
}

// Subscribe creates a new subscription
func (c *Client) Subscribe(ctx context.Context, payload map[string]interface{}) (string, error) {
	if err := c.connectWS(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	subscriptionID := strconv.Itoa(c.subscriptionIDCounter)
	c.subscriptionIDCounter++

	sub := Subscription{
		ID:      subscriptionID,
		Payload: payload,
	}
	if t, ok := payload["type"].(string); ok {
		sub.Type = t
	}
	c.subscriptions[subscriptionID] = sub
	c.mu.Unlock()

	msgJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal subscription payload: %w", err)
	}
	msg := fmt.Sprintf("sub %s %s", subscriptionID, msgJSON)

	err = c.ws.Write(c.wsContext, websocket.MessageText, []byte(msg))
	if err != nil {
		return "", fmt.Errorf("failed to send subscription: %w", err)
	}

	return subscriptionID, nil
}

// Unsubscribe removes a subscription
func (c *Client) Unsubscribe(_ context.Context, subscriptionID string) error {
	msg := fmt.Sprintf("unsub %s", subscriptionID)
	err := c.ws.Write(c.wsContext, websocket.MessageText, []byte(msg))

	c.mu.Lock()
	delete(c.subscriptions, subscriptionID)
	delete(c.previousResponses, subscriptionID)
	c.mu.Unlock()

	return err
}

// Recv returns the channel for receiving messages
func (c *Client) Recv() <-chan Message {
	return c.recvChan
}

// receiveLoop continuously receives messages from WebSocket
func (c *Client) receiveLoop() {
	defer close(c.recvChan)

	for {
		_, data, err := c.ws.Read(c.wsContext)
		if err != nil {
			if !errors.Is(c.wsContext.Err(), context.Canceled) {
				c.recvChan <- Message{Error: fmt.Errorf("websocket read error: %w", err)}
			}
			return
		}

		msg, err := c.parseMessage(string(data))
		if err != nil {
			c.recvChan <- Message{Error: fmt.Errorf("failed to parse message: %w", err)}
			return
		}
		if msg != nil {
			c.recvChan <- *msg
		}
	}
}

// TimelineDetailV2 subscribes to timeline detail
func (c *Client) TimelineDetailV2(ctx context.Context, timelineID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "timelineDetailV2",
		"id":   timelineID,
	})
}

// TimelineTransactions subscribes to timeline transactions
func (c *Client) TimelineTransactions(ctx context.Context, after *string) (string, error) {
	payload := map[string]interface{}{
		"type": "timelineTransactions",
	}
	if after != nil {
		payload["after"] = *after
	}
	return c.Subscribe(ctx, payload)
}
