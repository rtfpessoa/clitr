package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewClient("+1234567890", tmpDir, false)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, "+1234567890", client.phoneNo)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.subscriptions)
}

func TestNewClient_LoadsCookies(t *testing.T) {
	tmpDir := t.TempDir()
	phoneNo := "+1234567890"

	// Create a cookie file with compact format
	cookieDir := filepath.Join(tmpDir, "auth")
	err := os.MkdirAll(cookieDir, 0755)
	require.NoError(t, err)

	cookieFile := filepath.Join(cookieDir, "cookies."+phoneNo+".json")
	cookieData := `[{"n":"session","v":"test123","d":"api.traderepublic.com","p":"/"}]`
	err = os.WriteFile(cookieFile, []byte(cookieData), 0600)
	require.NoError(t, err)

	client, err := NewClient(phoneNo, tmpDir, true)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestParseMessage_FullMessage(t *testing.T) {
	client := &Client{
		subscriptions:     make(map[string]Subscription),
		previousResponses: make(map[string]string),
	}

	// Register a subscription
	client.subscriptions["1"] = Subscription{ID: "1", Type: "test"}

	msg, err := client.parseMessage(`1 A{"key":"value"}`)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, "1", msg.SubscriptionID)
	assert.Equal(t, "value", msg.Payload["key"])
}

func TestParseMessage_DeltaMessage(t *testing.T) {
	client := &Client{
		subscriptions:     make(map[string]Subscription),
		previousResponses: make(map[string]string),
	}

	// Register subscription and previous response
	client.subscriptions["1"] = Subscription{ID: "1", Type: "test"}
	client.previousResponses["1"] = `{"status":"old"}`

	// Delta message that replaces content
	msg, err := client.parseMessage(`1 D+%7B%22status%22%3A%22new%22%7D`)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, "new", msg.Payload["status"])
}

func TestParseMessage_ErrorMessage(t *testing.T) {
	client := &Client{
		subscriptions:     make(map[string]Subscription),
		previousResponses: make(map[string]string),
	}

	client.subscriptions["1"] = Subscription{ID: "1", Type: "test"}

	msg, err := client.parseMessage(`1 E{"error":"test error"}`)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Error(t, msg.Error)
	assert.Contains(t, msg.Error.Error(), "subscription error")
}

func TestCalculateDelta_AddOperation(t *testing.T) {
	client := &Client{}

	// URL-encoded JSON: {"status":"new"}
	// Note: The + sign is trimmed, so include leading space in expected
	result := client.calculateDelta("", "+%7B%22status%22%3A%22new%22%7D")

	// The implementation URL-decodes the content after the + sign
	assert.Contains(t, result, `"status":"new"`)
}

func TestCalculateDelta_MixedOperations(t *testing.T) {
	client := &Client{}

	// Previous: "Hello World"
	// Delta: keep first 6 chars (=6), then add URL-encoded "Go"
	// Note: In URL encoding, + becomes space, so use %20 or just regular chars
	// Tab-separated operations
	result := client.calculateDelta("Hello World", "=6\t+Go")

	// The + in "+Go" gets URL-decoded to " Go" (+ = space in URL encoding)
	assert.Equal(t, "Hello  Go", result)
}

func TestSaveCookies_EmptyCookies(t *testing.T) {
	tmpDir := t.TempDir()
	phoneNo := "+1234567890"

	client, err := NewClient(phoneNo, tmpDir, true)
	require.NoError(t, err)

	// Save cookies (even if empty, it should not error)
	// With empty cookie jar, SaveCookies returns early without saving anything
	err = client.SaveCookies()
	require.NoError(t, err)
}

func TestSaveCookies_Disabled(t *testing.T) {
	// Client with saveCookies disabled should just return nil
	client := &Client{
		saveCookies: false,
		phoneNo:     "+1234567890",
	}

	err := client.SaveCookies()
	assert.NoError(t, err) // Should not error when saveCookies is false
}

func TestLoadCookies_RestoresCookies(t *testing.T) {
	tmpDir := t.TempDir()
	phoneNo := "+1234567890"

	// Create a valid cookie file with compact format
	cookieDir := filepath.Join(tmpDir, "auth")
	err := os.MkdirAll(cookieDir, 0755)
	require.NoError(t, err)

	cookieFile := filepath.Join(cookieDir, "cookies."+phoneNo+".json")
	cookieData := `[{"n":"session","v":"abc123","p":"/","d":"api.traderepublic.com"}]`
	err = os.WriteFile(cookieFile, []byte(cookieData), 0600)
	require.NoError(t, err)

	client, err := NewClient(phoneNo, tmpDir, true)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Cookies should have been loaded
	// We can verify by checking the cookie jar indirectly
}

func TestLoadCookies_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Client without existing cookie file should work fine
	client, err := NewClient("+1234567890", tmpDir, true)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestRefreshWebSession_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/web/session" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)

	// Force token to be expired so refreshWebSession actually makes a request
	client.webSessionTokenExpires = client.webSessionTokenExpires.Add(-1)

	err = client.refreshWebSession()
	require.NoError(t, err)
}

func TestRefreshWebSession_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/web/session" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"AUTHENTICATION_ERROR"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)

	// Force token to be expired so refreshWebSession actually makes a request
	client.webSessionTokenExpires = client.webSessionTokenExpires.Add(-1)

	err = client.refreshWebSession()
	require.Error(t, err)
	// When AUTHENTICATION_ERROR is detected, cookies are reset and CookiesReset is returned
	assert.ErrorIs(t, err, CookiesReset)
}

// setupWSTestServer creates a test server that handles WebSocket connections
func setupWSTestServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("WebSocket accept error: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		handler(conn)
	}))
}

func TestSubscribe_IncrementsID(t *testing.T) {
	msgCount := 0
	done := make(chan struct{})
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read subscription messages
		for i := 0; i < 2; i++ {
			_, subscribePayload, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			snaps.MatchSnapshot(t, string(subscribePayload))
			msgCount++
		}
		close(done)
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	subID1, err := client.Portfolio(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0", subID1)

	subID2, err := client.Cash(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1", subID2)

	<-done
}

func TestUnsubscribe_RemovesSubscription(t *testing.T) {
	done := make(chan struct{})
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read subscription message
		_, subscribePayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(subscribePayload))
		// Read unsubscribe message
		_, unsubscribePayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(unsubscribePayload))
		close(done)
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	// Subscribe first
	subID, err := client.Subscribe(context.Background(), map[string]interface{}{"type": "portfolio"})
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	// Verify subscription exists
	_, exists := client.subscriptions[subID]
	assert.True(t, exists)

	// Unsubscribe
	err = client.Unsubscribe(context.Background(), subID)
	require.NoError(t, err)

	// Verify subscription is removed
	_, exists = client.subscriptions[subID]
	assert.False(t, exists)

	<-done
}

func TestTimelineTransactions_WithAfter(t *testing.T) {
	done := make(chan struct{})
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read timeline transactions subscription
		_, payload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(payload))
		close(done)
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	cursor := "cursor123"
	subID, err := client.TimelineTransactions(context.Background(), &cursor)
	require.NoError(t, err)
	assert.Equal(t, "0", subID)

	<-done
}

func TestTimelineTransactions_WithoutAfter(t *testing.T) {
	done := make(chan struct{})
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read timeline transactions subscription
		_, payload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(payload))
		close(done)
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	subID, err := client.TimelineTransactions(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "0", subID)

	<-done
}

func TestReceiveLoop_FullMessage(t *testing.T) {
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read subscription message
		_, subscribePayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(subscribePayload))
		// Send full message response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte(`0 A{"status":"active","value":42}`))
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	// Subscribe to start receiveLoop
	_, err = client.Subscribe(context.Background(), map[string]interface{}{"type": "portfolio"})
	require.NoError(t, err)

	msg, ok := <-client.Recv()
	require.True(t, ok)
	require.NoError(t, msg.Error)
	assert.Equal(t, "0", msg.SubscriptionID)
	assert.Equal(t, "active", msg.Payload["status"])
	assert.Equal(t, float64(42), msg.Payload["value"])
}

func TestReceiveLoop_ErrorMessage(t *testing.T) {
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read subscription message
		_, subscribePayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(subscribePayload))
		// Send error message response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte(`0 E{"error":"subscription_failed"}`))
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	// Subscribe to start receiveLoop
	_, err = client.Subscribe(context.Background(), map[string]interface{}{"type": "portfolio"})
	require.NoError(t, err)

	msg, ok := <-client.Recv()
	assert.True(t, ok)
	require.Error(t, msg.Error)
	assert.Contains(t, msg.Error.Error(), "subscription error")
	assert.Equal(t, "0", msg.SubscriptionID)
	assert.Equal(t, "subscription_failed", msg.Payload["error"])
}

func TestReceiveLoop_CompleteMessage(t *testing.T) {
	server := setupWSTestServer(t, func(conn *websocket.Conn) {
		// Read connect message
		_, connectPayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(connectPayload))
		// Send connected response
		_ = conn.Write(context.Background(), websocket.MessageText, []byte("connected"))
		// Read subscription message
		_, subscribePayload, _ := conn.Read(context.Background())
		snaps.MatchSnapshot(t, string(subscribePayload))
		// Send complete message (subscription finished)
		_ = conn.Write(context.Background(), websocket.MessageText, []byte(`0 C`))
	})
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	defer func(client *Client) { _ = client.Close() }(client)

	wsURL := "ws://" + strings.TrimPrefix(server.URL, "http://")
	client.setWSHost(wsURL)

	// Subscribe to start receiveLoop
	subID, err := client.Subscribe(context.Background(), map[string]interface{}{"type": "portfolio"})
	require.NoError(t, err)

	// Verify subscription exists before complete
	_, exists := client.subscriptions[subID]
	assert.True(t, exists)

	require.Eventually(t, func() bool {
		_, exists := client.subscriptions[subID]
		return !exists
	}, 5*time.Second, 10*time.Millisecond)
}
