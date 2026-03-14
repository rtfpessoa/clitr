// Package client provides interfaces for dependency injection in tests.
package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
)

// HTTPClient abstracts HTTP operations for testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// WebSocketConn abstracts WebSocket operations for testing.
type WebSocketConn interface {
	Write(ctx context.Context, typ websocket.MessageType, data []byte) error
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Close(code websocket.StatusCode, reason string) error
}

// CookieJar abstracts cookie operations for testing.
type CookieJar interface {
	Cookies(u *url.URL) []*http.Cookie
	SetCookies(u *url.URL, cookies []*http.Cookie)
}

// Ensure standard library types implement our interfaces.
var _ HTTPClient = (*http.Client)(nil)
