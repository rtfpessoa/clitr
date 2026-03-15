package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := ServerConfig{
		Host:       "127.0.0.1",
		Port:       0, // Will use httptest instead
		TrustProxy: false,
	}
	factory := func(phone string) (WebClient, error) {
		return newMockWebClient(), nil
	}
	srv, err := NewServer(cfg, factory)
	require.NoError(t, err)
	return srv
}

func TestServer_AllRoutesRegistered(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.handler)
	defer ts.Close()

	routes := []struct {
		method string
		path   string
		expect int
	}{
		{"GET", "/", http.StatusSeeOther},           // Redirects to /login
		{"GET", "/login", http.StatusOK},             // Renders login form
		{"GET", "/twofa", http.StatusSeeOther},       // No session → redirect
		{"GET", "/progress", http.StatusSeeOther},    // No session → redirect
		{"GET", "/result", http.StatusSeeOther},      // No session → redirect
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expect, resp.StatusCode, "route %s %s", tc.method, tc.path)
		})
	}
}

func TestServer_LoginRoute_SetsSecurityHeaders(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/login")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "nonce-")
}

func TestServer_GracefulShutdown(t *testing.T) {
	srv := newTestServer(t)

	// Start in a goroutine
	go func() {
		_ = srv.Start()
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestServer_RootRedirectsToLogin(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.handler)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestServer_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
