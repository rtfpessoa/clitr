package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitiateWebLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"processId":"test-process-123","countdownInSeconds":30}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)
	client.setStdinReader(strings.NewReader("1234\n")) // Mock PIN input

	countdown, err := client.initiateWebLogin("+1234567890")
	require.NoError(t, err)
	assert.Equal(t, 31, countdown) // countdownInSeconds + 1
	assert.Equal(t, "test-process-123", client.processID)
}

func TestInitiateWebLogin_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"errors":[{"errorCode":"INVALID_CREDENTIALS","errorMsg":"Invalid phone number or PIN"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)
	client.setStdinReader(strings.NewReader("0000\n")) // Mock wrong PIN

	_, err = client.initiateWebLogin("+1234567890")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INVALID_CREDENTIALS")
}

func TestCompleteWebLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login/test-process-id/1234" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)
	client.processID = "test-process-id"

	err = client.completeWebLogin("1234")
	require.NoError(t, err)
}

func TestCompleteWebLogin_NoProcessId(t *testing.T) {
	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	// processID is empty by default
	err = client.completeWebLogin("1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no process ID available")
}

func TestCompleteWebLogin_Invalid2FA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v1/auth/web/login/") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Invalid 2FA code"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	client.setAPIHost(server.URL)
	client.processID = "test-process-id"

	err = client.completeWebLogin("0000") // Wrong 2FA code
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web login verification failed with status 401")
}

func TestResumeWebSession_ValidSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// refreshWebSession endpoint
		if r.URL.Path == "/api/v1/auth/web/session" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Settings endpoint (called by resumeWebSession via Settings())
		if r.URL.Path == "/api/v2/auth/account" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"email":"test@example.com"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient("+1234567890", t.TempDir(), true) // saveCookies = true
	require.NoError(t, err)

	client.setAPIHost(server.URL)

	// Add a fake cookie to the jar so resumeWebSession doesn't return early
	client.httpClient.Jar.SetCookies(
		mustParseURL("https://api.traderepublic.com/api/v1/auth/web"),
		[]*http.Cookie{{Name: "session", Value: "test-session"}},
	)

	result := client.resumeWebSession()
	assert.True(t, result)
}

func TestResumeWebSession_InvalidSession(t *testing.T) {
	client, err := NewClient("+1234567890", t.TempDir(), false)
	require.NoError(t, err)

	// saveCookies is false, so resumeWebSession should return false
	result := client.resumeWebSession()
	assert.False(t, result)
}

func TestInitiateWebLoginWithCredentials_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"processId":"web-process-456","countdownInSeconds":25}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)

	c.setAPIHost(server.URL)

	// Call exported method directly — no stdin involved
	countdown, err := c.InitiateWebLoginWithCredentials("+4912345678", "1234")
	require.NoError(t, err)
	assert.Equal(t, 26, countdown) // countdownInSeconds + 1
	assert.Equal(t, "web-process-456", c.processID)
}

func TestInitiateWebLoginWithCredentials_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"errors":[{"errorCode":"INVALID_CREDENTIALS","errorMsg":"Invalid phone number or PIN"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)

	c.setAPIHost(server.URL)

	_, err = c.InitiateWebLoginWithCredentials("+4912345678", "0000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INVALID_CREDENTIALS")
}

func TestInitiateWebLoginWithCredentials_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal server error`))
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)

	c.setAPIHost(server.URL)

	_, err = c.InitiateWebLoginWithCredentials("+4912345678", "1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web login failed with status 500")
}

func TestCompleteWebLogin_Exported_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/auth/web/login/web-process-id/5678" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)

	c.setAPIHost(server.URL)
	c.processID = "web-process-id"

	err = c.CompleteWebLogin("5678")
	require.NoError(t, err)
}

func TestCompleteWebLogin_Exported_NoProcessId(t *testing.T) {
	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)

	err = c.CompleteWebLogin("1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no process ID available")
}

// Helper function to parse URL (panics on error, only for tests)
func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
