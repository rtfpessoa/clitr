package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders_AllPresent(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "nonce-")
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "script-src")
}

func TestSecurityHeaders_CSP_HasNonce(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the nonce is available in the request context
		nonce := NonceFromContext(r.Context())
		assert.NotEmpty(t, nonce)
		assert.Len(t, nonce, 32) // 16 bytes hex-encoded
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	// CSP should contain the nonce
	assert.Contains(t, csp, "script-src 'nonce-")
}

func TestSecurityHeaders_CSP_NonceDiffersPerRequest(t *testing.T) {
	var nonces []string

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonces = append(nonces, NonceFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	require.Len(t, nonces, 3)
	assert.NotEqual(t, nonces[0], nonces[1])
	assert.NotEqual(t, nonces[1], nonces[2])
}

func TestCSRFMiddleware_SkipsGET(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	called := false
	handler := CSRFMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_RejectsInvalidToken(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	handler := CSRFMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	body := strings.NewReader("csrf_token=wrong-token")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_AcceptsValidToken(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	called := false
	handler := CSRFMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader("csrf_token=" + session.CSRFToken)
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_RejectsNoSession(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	handler := CSRFMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	body := strings.NewReader("csrf_token=anything")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRateLimitMiddleware_AllowsUnderLimit(t *testing.T) {
	limiter := NewRateLimiter(5, 1*time.Minute)

	called := false
	handler := RateLimitMiddleware(limiter, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimitMiddleware_Blocks429(t *testing.T) {
	limiter := NewRateLimiter(2, 1*time.Minute)

	handler := RateLimitMiddleware(limiter, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// 3rd request should be rate-limited
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimitMiddleware_SkipsGET(t *testing.T) {
	limiter := NewRateLimiter(1, 1*time.Minute)

	called := 0
	handler := RateLimitMiddleware(limiter, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	// GET requests should not be rate-limited
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 5, called)
}

func TestBodySizeLimit_AllowsSmallPOST(t *testing.T) {
	called := false
	handler := BodySizeLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Read the body to verify it's accessible
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		assert.Greater(t, n, 0)
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader("csrf_token=abc&phone=+49123456789&pin=1234")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBodySizeLimit_RejectsOversizedPOST(t *testing.T) {
	handler := BodySizeLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attempt to parse form — this triggers the MaxBytesReader check
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body larger than maxBodyBytes (4096)
	largeBody := strings.Repeat("x", 5000)
	req := httptest.NewRequest("POST", "/login", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestBodySizeLimit_SkipsGET(t *testing.T) {
	called := false
	handler := BodySizeLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestLogging_SetsRequestID(t *testing.T) {
	handler := RequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is set in response header
		assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}
