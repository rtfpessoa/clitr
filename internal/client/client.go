// Package trclient provides a client for interacting with the Trade Republic API.
// It handles WebSocket connections, authentication, and subscription-based data retrieval.
package client

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/types"
	"github.com/zalando/go-keyring"
	"go.uber.org/zap"
)

const (
	defaultAPIHost = "https://api.traderepublic.com"
	cookiesHost    = "https://api.traderepublic.com/api/v1/auth/web"
	wsHost         = "wss://api.traderepublic.com"

	// Keyring constants for secure credential storage
	keyringService = "clitr"
	keyringKeyPfx  = "cookies."
)

var (
	ErrCookiesReset = errors.New("cookies reset")
)

// closeBody closes an io.ReadCloser, logging any error. Use with defer on response bodies.
func closeBody(body io.ReadCloser) {
	if err := body.Close(); err != nil {
		log.Error("failed to close response body", zap.Error(err))
	}
}

// Client represents a Trade Republic API client
type Client struct {
	phoneNo string

	httpClient *http.Client
	ws         *websocket.Conn
	wsContext  context.Context
	wsCancel   context.CancelFunc

	processID              string
	webSessionTokenExpires time.Time

	dataDir     string // data directory for legacy cookie migration
	saveCookies bool

	subscriptionIDCounter int
	subscriptions         map[string]Subscription
	previousResponses     map[string]string
	mu                    sync.Mutex

	recvChan chan Message

	// apiHost is the base URL for API requests (configurable for testing)
	apiHost string
	// wsHost is the WebSocket host URL (configurable for testing)
	wsHost string
	// stdinReader is the reader for stdin input (configurable for testing)
	stdinReader io.Reader
}

// Subscription represents a WebSocket subscription
type Subscription struct {
	ID      string
	Type    string
	Payload map[string]interface{}
}

// Message represents a received WebSocket message
type Message struct {
	SubscriptionID string
	Subscription   Subscription
	Payload        map[string]interface{}
	Error          error
}

// NewClient creates a new Trade Republic API client
func NewClient(phoneNo string, dataDir string, saveCookies bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &Client{
		phoneNo:           phoneNo,
		httpClient:        &http.Client{Jar: jar, Timeout: 30 * time.Second},
		subscriptions:     make(map[string]Subscription),
		previousResponses: make(map[string]string),
		saveCookies:       saveCookies,
		dataDir:           dataDir,
		recvChan:          make(chan Message, 100),
		apiHost:           defaultAPIHost,
		wsHost:            wsHost,
		stdinReader:       os.Stdin,
	}

	// Try to load saved cookies if enabled
	if saveCookies {
		err := client.loadCookies()
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

// setAPIHost sets the API host for testing purposes
func (c *Client) setAPIHost(host string) {
	c.apiHost = host
}

// setStdinReader sets the stdin reader for testing purposes
func (c *Client) setStdinReader(reader io.Reader) {
	c.stdinReader = reader
}

// setWSHost sets the WebSocket host for testing purposes
func (c *Client) setWSHost(host string) {
	c.wsHost = host
}

func (c *Client) AuthenticateClient() error {
	log.Info("Connecting to Trade Republic")

	// Try to resume existing session if cookies are saved
	if c.saveCookies && c.resumeWebSession() {
		log.Info("Resumed existing session from saved cookies")
		return nil
	}

	// Initiate web login
	log.Info("Initiating login")
	countdown, err := c.initiateWebLogin(c.phoneNo)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Prompt for 2FA code
	fmt.Printf("A 4-digit code has been sent to your Trade Republic app.\n")
	fmt.Printf("Please enter the code (valid for %d seconds): ", countdown)

	reader := bufio.NewReader(os.Stdin)
	codeInput, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read 2FA code: %w", err)
	}
	code := strings.TrimSpace(codeInput)

	// Complete login
	if err := c.completeWebLogin(code); err != nil {
		return fmt.Errorf("2FA verification failed: %w", err)
	}
	log.Info("Login successful")

	return nil
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

	headers := http.Header{}
	// Add cookies from HTTP client
	// Parse both HTTP and WebSocket URLs to get cookies
	httpURL, err := url.Parse(cookiesHost)
	if err != nil {
		return fmt.Errorf("failed to parse cookies host: %w", err)
	}
	wsURL, err := url.Parse(c.wsHost)
	if err != nil {
		return fmt.Errorf("failed to parse websocket host: %w", err)
	}

	// Try to get cookies for both URLs
	httpCookies := c.httpClient.Jar.Cookies(httpURL)
	wsCookies := c.httpClient.Jar.Cookies(wsURL)

	// Merge cookies (prefer WS cookies if duplicates)
	cookieMap := make(map[string]string)
	for _, cookie := range httpCookies {
		if strings.Contains(cookie.Domain, "traderepublic.com") || cookie.Domain == "" {
			cookieMap[cookie.Name] = cookie.Value
		}
	}
	for _, cookie := range wsCookies {
		if strings.Contains(cookie.Domain, "traderepublic.com") || cookie.Domain == "" {
			cookieMap[cookie.Name] = cookie.Value
		}
	}

	var cookieStrs []string
	for name, value := range cookieMap {
		cookieStrs = append(cookieStrs, fmt.Sprintf("%s=%s", name, value))
	}

	if len(cookieStrs) > 0 {
		cookieHeader := strings.Join(cookieStrs, "; ")
		headers.Set("Cookie", cookieHeader)
	}

	conn, resp, err := websocket.Dial(ctx, c.wsHost, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	c.ws = conn
	c.wsContext, c.wsCancel = context.WithCancel(ctx)

	// Send connection message
	var connectMsg map[string]interface{}
	var webSocketApiVersion int
	connectMsg = map[string]interface{}{
		"locale":          types.DefaultLanguage,
		"platformId":      "webtrading",
		"platformVersion": "chrome - 142.0.0",
		"clientId":        "app.traderepublic.com",
		"clientVersion":   "11.4.1",
	}
	webSocketApiVersion = 33

	msgJSON, err := json.Marshal(connectMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %v", connectMsg)
	}
	msg := fmt.Sprintf("connect %d %s", webSocketApiVersion, msgJSON)

	err = c.ws.Write(c.wsContext, websocket.MessageText, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send connect message: %w", err)
	}

	// Read connection response
	msgType, response, err := c.ws.Read(c.wsContext)
	if err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if msgType != websocket.MessageText {
		return fmt.Errorf("expected text message, got %s", msgType)
	}

	if string(response) != "connected" {
		return fmt.Errorf("connection failed with message type '%s': %s", msgType, string(response))
	}

	// Start message receiver goroutine
	go c.receiveLoop()

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

// parseMessage parses a WebSocket message
func (c *Client) parseMessage(data string) (*Message, error) {
	// Format: "<subscription_id> <code><payload>"
	parts := strings.SplitN(data, " ", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("failed to parse message: %s", data)
	}

	subscriptionID := parts[0]
	rest := parts[1]

	if len(rest) == 0 {
		return nil, fmt.Errorf("failed to parse message: %s", data)
	}

	code := rest[0]
	payloadStr := ""
	if len(rest) > 1 {
		payloadStr = rest[1:]
	}

	c.mu.Lock()
	sub, ok := c.subscriptions[subscriptionID]
	c.mu.Unlock()

	if !ok && code != 'C' {
		// No active subscription
		return nil, fmt.Errorf("no active subscription: %s", data)
	}

	switch code {
	case 'A': // Full message
		c.mu.Lock()
		c.previousResponses[subscriptionID] = payloadStr
		c.mu.Unlock()

		var payload map[string]interface{}
		if payloadStr != "" {
			err := json.Unmarshal([]byte(payloadStr), &payload)
			if err != nil {
				return nil, err
			}
		}

		return &Message{
			SubscriptionID: subscriptionID,
			Subscription:   sub,
			Payload:        payload,
		}, nil

	case 'D': // Delta message
		c.mu.Lock()
		previous := c.previousResponses[subscriptionID]
		c.mu.Unlock()

		result := c.calculateDelta(previous, payloadStr)

		c.mu.Lock()
		c.previousResponses[subscriptionID] = result
		c.mu.Unlock()

		var payload map[string]interface{}
		if result != "" {
			err := json.Unmarshal([]byte(result), &payload)
			if err != nil {
				return nil, err
			}
		}

		return &Message{
			SubscriptionID: subscriptionID,
			Subscription:   sub,
			Payload:        payload,
		}, nil

	case 'C': // Complete
		c.mu.Lock()
		delete(c.subscriptions, subscriptionID)
		delete(c.previousResponses, subscriptionID)
		c.mu.Unlock()
		return nil, nil

	case 'E': // Error
		c.mu.Lock()
		delete(c.subscriptions, subscriptionID)
		delete(c.previousResponses, subscriptionID)
		c.mu.Unlock()

		var payload map[string]interface{}
		if payloadStr != "" {
			err := json.Unmarshal([]byte(payloadStr), &payload)
			if err != nil {
				return nil, err
			}
		}

		return &Message{
			SubscriptionID: subscriptionID,
			Subscription:   sub,
			Payload:        payload,
			Error:          fmt.Errorf("subscription error: %s", payloadStr),
		}, nil
	}

	return nil, nil
}

// calculateDelta applies delta compression
func (c *Client) calculateDelta(previous, delta string) string {
	parts := strings.Split(delta, "\t")
	var result strings.Builder
	i := 0

	for _, part := range parts {
		if len(part) == 0 {
			continue
		}

		sign := part[0]
		switch sign {
		case '+':
			decoded, err := url.QueryUnescape(strings.TrimSpace(part))
			if err != nil {
				log.Error("failed to unescape delta part", zap.Error(err))
				return ""
			}
			result.WriteString(decoded)
		case '-', '=':
			if len(part) < 2 {
				continue
			}
			length, err := strconv.Atoi(part[1:])
			if err != nil {
				log.Error("failed to convert delta part to int", zap.Error(err))
				return ""
			}
			if sign == '=' {
				if i+length <= len(previous) {
					result.WriteString(previous[i : i+length])
				}
			}
			i += length
		}
	}

	return result.String()
}

// savedCookie contains only essential cookie fields to minimize storage size (compact format)
type savedCookie struct {
	Name     string    `json:"n"`
	Value    string    `json:"v"`
	Path     string    `json:"p,omitempty"`
	Domain   string    `json:"d,omitempty"`
	Expires  time.Time `json:"e,omitempty"`
	Secure   bool      `json:"s,omitempty"`
	HttpOnly bool      `json:"h,omitempty"`
	SameSite int       `json:"ss,omitempty"`
}

// TimelineDetailV2 subscribes to timeline detail
func (c *Client) TimelineDetailV2(ctx context.Context, timelineID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "timelineDetailV2",
		"id":   timelineID,
	})
}

func (c *Client) ResetCookies() error {
	keyringKey := keyringKeyPfx + c.phoneNo
	if err := keyring.Delete(keyringService, keyringKey); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil // Nothing to delete
		}
		return fmt.Errorf("failed to delete cookies from keyring: %w", err)
	}
	return nil
}

// SaveCookies saves cookies to the system keyring (compressed)
func (c *Client) SaveCookies() error {
	if !c.saveCookies {
		return nil
	}

	parsedURL, err := url.Parse(cookiesHost)
	if err != nil {
		return err
	}
	cookies := c.httpClient.Jar.Cookies(parsedURL)

	if len(cookies) == 0 {
		// No cookies to save - this is normal on first run
		return nil
	}

	// Store only essential cookie fields
	var cookieList []savedCookie
	for _, cookie := range cookies {
		cookieList = append(cookieList, savedCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
			SameSite: int(cookie.SameSite),
		})
	}

	jsonData, err := json.Marshal(cookieList)
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	// Compress with gzip to reduce size for keyring storage limits
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	if _, err := gzWriter.Write(jsonData); err != nil {
		return fmt.Errorf("failed to compress cookies: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize compression: %w", err)
	}

	// Base64 encode for safe string storage
	encoded := base64.StdEncoding.EncodeToString(compressed.Bytes())

	// Store in system keyring
	keyringKey := keyringKeyPfx + c.phoneNo
	if err := keyring.Set(keyringService, keyringKey, encoded); err != nil {
		return fmt.Errorf("failed to save cookies to keyring: %w", err)
	}

	return nil
}

// loadCookies loads cookies from system keyring, with fallback to legacy file for migration
func (c *Client) loadCookies() error {
	keyringKey := keyringKeyPfx + c.phoneNo

	// Try keyring first
	data, err := keyring.Get(keyringService, keyringKey)
	if err == nil {
		// Found in keyring - decompress and parse
		jsonData, decompressErr := decompressCookieData(data)
		if decompressErr != nil {
			log.Debug("failed to decompress keyring data, trying as plain JSON", zap.Error(decompressErr))
			// Try as plain JSON (old format)
			jsonData = []byte(data)
		}
		return c.parseCookiesIntoJar(jsonData)
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		// Keyring error (not just "not found") - log and continue to legacy fallback
		log.Debug("failed to load from keyring, trying legacy file", zap.Error(err))
	}

	// Fallback: check for legacy file
	legacyPath := filepath.Join(c.dataDir, "auth", fmt.Sprintf("cookies.%s.json", c.phoneNo))
	fileData, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cookies anywhere
		}
		return fmt.Errorf("failed to read legacy cookie file: %w", err)
	}

	// Parse legacy file first to validate it
	if parseErr := c.parseCookiesIntoJar(fileData); parseErr != nil {
		return fmt.Errorf("failed to parse legacy cookies: %w", parseErr)
	}

	// Re-save to keyring using new compressed format
	if saveErr := c.SaveCookies(); saveErr != nil {
		log.Warn("failed to migrate cookies to keyring, keeping legacy file", zap.Error(saveErr))
		return nil
	}

	// Migration successful - delete legacy file
	if removeErr := os.Remove(legacyPath); removeErr != nil {
		log.Warn("failed to remove legacy cookie file after migration", zap.Error(removeErr))
	} else {
		log.Info("migrated cookies from file to system keyring")
	}

	return nil
}

// decompressCookieData decodes base64 and decompresses gzip data
func decompressCookieData(encoded string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	return decompressed, nil
}

// parseCookiesIntoJar parses cookie JSON data and sets cookies in the HTTP client jar
func (c *Client) parseCookiesIntoJar(data []byte) error {
	var cookieList []savedCookie
	if err := stdjson.Unmarshal(data, &cookieList); err != nil {
		return fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	if len(cookieList) == 0 {
		return nil
	}

	parsedURL, err := url.Parse(cookiesHost)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	var cookies []*http.Cookie
	for _, sc := range cookieList {
		cookies = append(cookies, &http.Cookie{
			Name:     sc.Name,
			Value:    sc.Value,
			Path:     sc.Path,
			Domain:   sc.Domain,
			Expires:  sc.Expires,
			Secure:   sc.Secure,
			HttpOnly: sc.HttpOnly,
			SameSite: http.SameSite(sc.SameSite),
		})
	}

	c.httpClient.Jar.SetCookies(parsedURL, cookies)
	return nil
}

// RefreshWebSession refreshes the web session token
func (c *Client) refreshWebSession() error {
	if time.Now().Before(c.webSessionTokenExpires) {
		return nil // Token still valid
	}

	req, err := http.NewRequest("GET", c.apiHost+"/api/v1/auth/web/session", nil)
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentHeaderValue)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		bodyText, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error("failed to read response body", zap.Error(err))
		}
		if strings.Contains(string(bodyText), "AUTHENTICATION_ERROR") {
			err = c.ResetCookies()
			if err == nil {
				log.Debug("cookies reset successfully")
				return ErrCookiesReset
			}
			log.Error("failed to reset cookies", zap.Error(err))
		}
		return fmt.Errorf("session refresh failed with status %d: %s", resp.StatusCode, bodyText)
	}

	c.webSessionTokenExpires = time.Now().Add(290 * time.Second)
	return nil
}

// WebRequest makes an authenticated web request
func (c *Client) webRequest(method, path string) (*http.Response, error) {
	if err := c.refreshWebSession(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.apiHost+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgentHeaderValue)

	return c.httpClient.Do(req)
}

// Settings retrieves account settings
func (c *Client) Settings() (map[string]interface{}, error) {
	resp, err := c.webRequest("GET", "/api/v2/auth/account")
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("settings request failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode settings: %w", err)
	}

	return result, nil
}

// Portfolio subscribes to portfolio
func (c *Client) Portfolio(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "portfolio",
	})
}

// Cash subscribes to cash information
func (c *Client) Cash(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "cash",
	})
}

// InstrumentDetails subscribes to instrument details
func (c *Client) InstrumentDetails(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "instrument",
		"id":   isin,
	})
}

// PortfolioStatus subscribes to portfolio status
func (c *Client) PortfolioStatus(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "portfolioStatus",
	})
}

// CompactPortfolio subscribes to compact portfolio
func (c *Client) CompactPortfolio(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "compactPortfolio",
	})
}

// Watchlist subscribes to watchlist
func (c *Client) Watchlist(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "watchlist",
	})
}

// AvailableCashForPayout subscribes to available cash for payout
func (c *Client) AvailableCashForPayout(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "availableCashForPayout",
	})
}

// PortfolioHistory subscribes to portfolio history
func (c *Client) PortfolioHistory(ctx context.Context, timeframe string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":  "portfolioAggregateHistory",
		"range": timeframe,
	})
}

// InstrumentSuitability subscribes to instrument suitability
func (c *Client) InstrumentSuitability(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "instrumentSuitability",
		"instrumentId": isin,
	})
}

// StockDetails subscribes to stock details
func (c *Client) StockDetails(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "stockDetails",
		"id":   isin,
	})
}

// AddWatchlist adds an instrument to watchlist
func (c *Client) AddWatchlist(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "addToWatchlist",
		"instrumentId": isin,
	})
}

// RemoveWatchlist removes an instrument from watchlist
func (c *Client) RemoveWatchlist(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "removeFromWatchlist",
		"instrumentId": isin,
	})
}

// Ticker subscribes to ticker
func (c *Client) Ticker(ctx context.Context, isin, exchange string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "ticker",
		"id":   fmt.Sprintf("%s.%s", isin, exchange),
	})
}

// Performance subscribes to performance
func (c *Client) Performance(ctx context.Context, isin, exchange string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "performance",
		"id":   fmt.Sprintf("%s.%s", isin, exchange),
	})
}

// PerformanceHistory subscribes to performance history
func (c *Client) PerformanceHistory(ctx context.Context, isin, timeframe, exchange string, resolution *int) (string, error) {
	params := map[string]interface{}{
		"type":  "aggregateHistory",
		"id":    fmt.Sprintf("%s.%s", isin, exchange),
		"range": timeframe,
	}
	if resolution != nil {
		params["resolution"] = *resolution
	}
	return c.Subscribe(ctx, params)
}

// Experience subscribes to experience
func (c *Client) Experience(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "experience",
	})
}

// MessageOfTheDay subscribes to message of the day
func (c *Client) MessageOfTheDay(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "messageOfTheDay",
	})
}

// NeonCards subscribes to neon cards
func (c *Client) NeonCards(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "neonCards",
	})
}

// TimelineDetail subscribes to timeline detail (v1)
func (c *Client) TimelineDetail(ctx context.Context, timelineID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "timelineDetail",
		"id":   timelineID,
	})
}

// TimelineDetailOrder subscribes to timeline detail for order
func (c *Client) TimelineDetailOrder(ctx context.Context, orderID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":    "timelineDetail",
		"orderId": orderID,
	})
}

// TimelineDetailSavingsPlan subscribes to timeline detail for savings plan
func (c *Client) TimelineDetailSavingsPlan(ctx context.Context, savingsPlanID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":          "timelineDetail",
		"savingsPlanId": savingsPlanID,
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

// TimelineActivityLog subscribes to timeline activity log
func (c *Client) TimelineActivityLog(ctx context.Context, after *string) (string, error) {
	payload := map[string]interface{}{
		"type": "timelineActivityLog",
	}
	if after != nil {
		payload["after"] = *after
	}
	return c.Subscribe(ctx, payload)
}

// SearchTags subscribes to neon search tags
func (c *Client) SearchTags(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "neonSearchTags",
	})
}

// SearchSuggestedTags subscribes to neon search suggested tags
func (c *Client) SearchSuggestedTags(ctx context.Context, query string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "neonSearchSuggestedTags",
		"data": map[string]interface{}{
			"q": query,
		},
	})
}

// Search performs a search
func (c *Client) Search(ctx context.Context, query, assetType string, page, pageSize int, aggregate, onlySavable bool, filterIndex, filterCountry, filterSector, filterRegion *string) (string, error) {
	filters := []map[string]string{
		{"key": "type", "value": assetType},
	}
	if onlySavable {
		filters = append(filters, map[string]string{"key": "attribute", "value": "savable"})
	}
	if filterIndex != nil {
		filters = append(filters, map[string]string{"key": "index", "value": *filterIndex})
	}
	if filterCountry != nil {
		filters = append(filters, map[string]string{"key": "country", "value": *filterCountry})
	}
	if filterRegion != nil {
		filters = append(filters, map[string]string{"key": "region", "value": *filterRegion})
	}
	if filterSector != nil {
		filters = append(filters, map[string]string{"key": "sector", "value": *filterSector})
	}

	searchType := "neonSearch"
	if aggregate {
		searchType = "neonSearchAggregations"
	}

	return c.Subscribe(ctx, map[string]interface{}{
		"type": searchType,
		"data": map[string]interface{}{
			"q":        query,
			"filter":   filters,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// SearchDerivative subscribes to derivatives search
func (c *Client) SearchDerivative(ctx context.Context, underlyingISIN, productType string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":            "derivatives",
		"underlying":      underlyingISIN,
		"productCategory": productType,
	})
}

// OrderOverview subscribes to orders overview
func (c *Client) OrderOverview(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "orders",
	})
}

// PriceForOrder subscribes to price for order
func (c *Client) PriceForOrder(ctx context.Context, isin, exchange, orderType string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "priceForOrder",
		"parameters": map[string]interface{}{
			"exchangeId":   exchange,
			"instrumentId": isin,
			"type":         orderType,
		},
	})
}

// CashAvailableForOrder subscribes to available cash
func (c *Client) CashAvailableForOrder(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "availableCash",
	})
}

// SizeAvailableForOrder subscribes to available size
func (c *Client) SizeAvailableForOrder(ctx context.Context, isin, exchange string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "availableSize",
		"parameters": map[string]interface{}{
			"exchangeId":   exchange,
			"instrumentId": isin,
		},
	})
}

// CancelOrder subscribes to cancel order
func (c *Client) CancelOrder(ctx context.Context, orderID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":    "cancelOrder",
		"orderId": orderID,
	})
}

// SavingsPlanOverview subscribes to savings plans overview
func (c *Client) SavingsPlanOverview(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "savingsPlans",
	})
}

// CancelSavingsPlan subscribes to cancel savings plan
func (c *Client) CancelSavingsPlan(ctx context.Context, savingsPlanID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "cancelSavingsPlan",
		"id":   savingsPlanID,
	})
}

// PriceAlarmOverview subscribes to price alarms overview
func (c *Client) PriceAlarmOverview(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "priceAlarms",
	})
}

// CreatePriceAlarm subscribes to create price alarm
func (c *Client) CreatePriceAlarm(ctx context.Context, isin string, price float64) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "createPriceAlarm",
		"instrumentId": isin,
		"targetPrice":  price,
	})
}

// CancelPriceAlarm subscribes to cancel price alarm
func (c *Client) CancelPriceAlarm(ctx context.Context, priceAlarmID string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "cancelPriceAlarm",
		"id":   priceAlarmID,
	})
}

// News subscribes to news for an instrument
func (c *Client) News(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "neonNews",
		"isin": isin,
	})
}

// NewsSubscriptions subscribes to news subscriptions
func (c *Client) NewsSubscriptions(ctx context.Context) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type": "newsSubscriptions",
	})
}

// SubscribeNews subscribes to news for an instrument
func (c *Client) SubscribeNews(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "subscribeNews",
		"instrumentId": isin,
	})
}

// UnsubscribeNews unsubscribes from news for an instrument
func (c *Client) UnsubscribeNews(ctx context.Context, isin string) (string, error) {
	return c.Subscribe(ctx, map[string]interface{}{
		"type":         "unsubscribeNews",
		"instrumentId": isin,
	})
}
