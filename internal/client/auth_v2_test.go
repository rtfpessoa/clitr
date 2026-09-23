package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebLoginV2PushApproval(t *testing.T) {
	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertV2RequestHeaders(t, r)

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/auth/web/login":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "+4912345678", body["phoneNumber"])
			assert.Equal(t, "1234", body["pin"])
			_, _ = w.Write([]byte(`{"processId":"process-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/auth/web/login/processes/process-1":
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"status":"PENDING"}`))
			} else {
				w.Header().Set("Set-Cookie", "JSESSIONID=test-session; Path=/")
				_, _ = w.Write([]byte(`{"status":"CONFIRMED"}`))
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)

	challenge, err := c.InitiateWebLoginV2("+4912345678", "1234")
	require.NoError(t, err)
	assert.False(t, challenge.RequiresAuthenticator)
	assert.Positive(t, challenge.Countdown)
	require.NoError(t, c.CompleteWebLoginV2(context.Background(), ""))
	assert.Equal(t, 2, polls)
	assert.Equal(t, "test-session", c.httpClient.Jar.Cookies(mustParseURL(server.URL))[0].Value)
}

func assertV2RequestHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	assert.Equal(t, "web-pro", r.Header.Get("X-TR-Platform"))
	assert.NotEmpty(t, r.Header.Get("X-TR-App-Version"))
	encoded := r.Header.Get("X-TR-Device-Info")
	deviceJSON, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	var device map[string]any
	require.NoError(t, json.Unmarshal(deviceJSON, &device))
	assert.NotEmpty(t, device["stableDeviceId"])
}

func TestWebLoginV2Authenticator(t *testing.T) {
	var submittedCode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/web/login":
			_, _ = w.Write([]byte(`{"processId":"process-2"}`))
		case "/api/v2/auth/web/login/processes/process-2":
			_, _ = w.Write([]byte(`{"status":"PENDING","requiredAction":"AUTHENTICATOR_VERIFICATION"}`))
		case "/api/v2/auth/web/login/processes/process-2/authenticator-verification":
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			submittedCode = body["code"]
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	challenge, err := c.InitiateWebLoginV2("+4912345678", "1234")
	require.NoError(t, err)
	assert.True(t, challenge.RequiresAuthenticator)
	require.NoError(t, c.CompleteWebLoginV2(context.Background(), "123456"))
	assert.Equal(t, "123456", submittedCode)
}

func TestWebLoginV2ReportsWAFRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "awselb/2.0")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	_, err = c.InitiateWebLoginV2("+4912345678", "1234")
	require.Error(t, err)
	assert.True(t, strings.Contains(strings.ToLower(err.Error()), "waf"))
}

func TestWebLoginV2ReportsApplicationErrorOnSuccessfulHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"errorCode":"PIN_INVALID"}]}`))
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	_, err = c.InitiateWebLoginV2("+4912345678", "1234")
	require.ErrorContains(t, err, "PIN_INVALID")
}

func TestAuthenticateClientUsesV2PushApproval(t *testing.T) {
	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/web/login":
			_, _ = w.Write([]byte(`{"processId":"process-3"}`))
		case "/api/v2/auth/web/login/processes/process-3":
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"status":"PENDING"}`))
			} else {
				_, _ = w.Write([]byte(`{"status":"CONFIRMED"}`))
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	c.setStdinReader(strings.NewReader("1234\n"))
	require.NoError(t, c.AuthenticateClient())
	assert.Equal(t, 2, polls)
}

func TestAuthenticateClientUsesPromptedPhoneNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/web/login":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "+4912345678", body["phoneNumber"])
			_, _ = w.Write([]byte(`{"processId":"process-4"}`))
		case "/api/v2/auth/web/login/processes/process-4":
			_, _ = w.Write([]byte(`{"status":"CONFIRMED"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClient("", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	c.setStdinReader(strings.NewReader("+4912345678\n1234\n"))
	require.NoError(t, c.AuthenticateClient())
	assert.Equal(t, "+4912345678", c.phoneNo)
}

func TestAuthenticateClientReadsAuthenticatorCodeAfterPIN(t *testing.T) {
	var gotCode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/web/login":
			_, _ = w.Write([]byte(`{"processId":"process-5"}`))
		case "/api/v2/auth/web/login/processes/process-5":
			_, _ = w.Write([]byte(`{"status":"PENDING","requiredAction":"AUTHENTICATOR_VERIFICATION"}`))
		case "/api/v2/auth/web/login/processes/process-5/authenticator-verification":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			gotCode = body["code"]
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClient("+4912345678", t.TempDir(), false)
	require.NoError(t, err)
	c.setAPIHost(server.URL)
	c.setStdinReader(strings.NewReader("1234\n123456\n"))
	require.NoError(t, c.AuthenticateClient())
	assert.Equal(t, "123456", gotCode)
}
