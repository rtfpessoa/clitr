package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResumeWebSession_ValidSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/web/session" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v2/auth/account" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"email":"test@example.com"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient("+1234567890", t.TempDir(), true)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	c.httpClient.Jar.SetCookies(
		mustParseURL("https://api.traderepublic.com/api/v1/auth/web"),
		[]*http.Cookie{{Name: "session", Value: "test-session"}},
	)
	assert.True(t, c.resumeWebSession())
}

func TestResumeWebSession_InvalidSession(t *testing.T) {
	c, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)
	assert.False(t, c.resumeWebSession())
}

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
