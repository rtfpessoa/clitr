package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// sessionCookieName is the cookie name used to identify sessions.
const sessionCookieName = "session_id"

// nonceKey is the context key for storing the CSP nonce.
type nonceKey struct{}

// NonceFromContext retrieves the CSP nonce from the request context.
// Returns empty string if no nonce is set.
func NonceFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(nonceKey{}).(string); ok {
		return v
	}
	return ""
}

// SecurityHeaders adds security headers to every response.
// It generates a per-request nonce for Content-Security-Policy (CSP)
// and stores it in the request context for template rendering.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate per-request nonce for CSP
		nonce := generateNonce()

		// Store nonce in context for templates
		ctx := context.WithValue(r.Context(), nonceKey{}, nonce)
		r = r.WithContext(ctx)

		// Set security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			fmt.Sprintf("default-src 'none'; script-src 'nonce-%s'; style-src 'nonce-%s'; connect-src 'self'; form-action 'self'; base-uri 'self'", nonce, nonce))

		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware validates CSRF tokens on POST requests.
// GET, HEAD, and OPTIONS requests are passed through without validation.
// The session is looked up from the session cookie.
func CSRFMiddleware(store *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF for safe methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Look up session
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			session := store.Get(cookie.Value)
			if session == nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Parse form to get CSRF token
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			formToken := r.FormValue("csrf_token")
			if !ValidateCSRFToken(session, formToken) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware limits POST requests per client IP.
// GET requests are not rate-limited.
// When trustProxy is true, the client IP is extracted from X-Forwarded-For.
func RateLimitMiddleware(limiter *RateLimiter, trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only rate-limit POST requests (form submissions)
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			ip := ClientIP(r, trustProxy)
			if !limiter.Allow(ip) {
				log.Debug("Rate limit exceeded", zap.String("ip", ip))
				http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogging adds a unique request ID and logs each request.
func RequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := generateRequestID()
		w.Header().Set("X-Request-Id", requestID)

		log.Debug("HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("request_id", requestID),
		)

		next.ServeHTTP(w, r)
	})
}

// generateNonce creates a 16-byte random nonce, hex-encoded (32 chars).
func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// generateRequestID creates a short unique request identifier.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
