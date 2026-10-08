// Package client provides a client for interacting with the Trade Republic API.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const (
	defaultAPIHost = "https://api.traderepublic.com"
	// cookiesHost is a bare-origin URL (no path) used solely to query/restore
	// cookies from the jar. http.CookieJar.Cookies() only ever returns a
	// cookie's Name/Value (Path, Domain, Expires are not exposed), so on
	// reload SetCookies() falls back to RFC 6265's default-path algorithm,
	// which derives the path from this URL. Trade Republic's session cookies
	// are actually scoped to Path=/, so this must have an empty path too -
	// otherwise reloaded cookies get scoped to whatever path segment happens
	// to be here and silently stop being sent on most requests after the
	// process restarts, forcing an unnecessary full re-login.
	cookiesHost = "https://api.traderepublic.com"
	wsHost      = "wss://api.traderepublic.com"

	// Keyring constants for secure credential storage
	keyringService = "clitr"
	keyringKeyPfx  = "cookies."
)

var (
	ErrCookiesReset = errors.New("cookies reset")
	ErrRateLimited  = errors.New("rate limited by Trade Republic")
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

	processID               string
	v2DeviceInfo            string
	v2RequiresAuthenticator bool
	v2Deadline              time.Time
	webSessionTokenExpires  time.Time

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
