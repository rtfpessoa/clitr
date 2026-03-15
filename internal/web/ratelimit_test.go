package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)

	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("192.168.1.1"), "request %d should be allowed", i+1)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)

	// Use up the limit
	for i := 0; i < 5; i++ {
		rl.Allow("192.168.1.1")
	}

	// 6th request should be blocked
	assert.False(t, rl.Allow("192.168.1.1"))
}

func TestRateLimiter_DifferentIPs_Independent(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)

	// Fill up one IP
	for i := 0; i < 5; i++ {
		rl.Allow("192.168.1.1")
	}
	assert.False(t, rl.Allow("192.168.1.1"))

	// Another IP should still be allowed
	assert.True(t, rl.Allow("192.168.1.2"))
}

func TestRateLimiter_WindowExpires(t *testing.T) {
	// Use a very short window
	rl := NewRateLimiter(2, 5*time.Millisecond)

	assert.True(t, rl.Allow("10.0.0.1"))
	assert.True(t, rl.Allow("10.0.0.1"))
	assert.False(t, rl.Allow("10.0.0.1"))

	// Wait for window to expire
	time.Sleep(10 * time.Millisecond)

	// Should be allowed again
	assert.True(t, rl.Allow("10.0.0.1"))
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Millisecond)

	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.2")

	time.Sleep(5 * time.Millisecond)

	removed := rl.Cleanup()
	assert.Equal(t, 2, removed)
}

func TestRateLimiter_Cleanup_KeepsActive(t *testing.T) {
	rl := NewRateLimiter(5, 10*time.Minute)

	rl.Allow("10.0.0.1")

	removed := rl.Cleanup()
	assert.Equal(t, 0, removed)
}

// --- IP extraction tests ---

func TestClientIP_DirectConnection(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	// No trusted proxy: use RemoteAddr
	ip := ClientIP(req, false)
	assert.Equal(t, "192.168.1.100", ip)
}

func TestClientIP_TrustedProxy_XForwardedFor(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")

	// Trusted proxy: use leftmost IP from X-Forwarded-For
	ip := ClientIP(req, true)
	assert.Equal(t, "203.0.113.50", ip)
}

func TestClientIP_TrustedProxy_NoHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	// Trusted proxy but no X-Forwarded-For: fall back to RemoteAddr
	ip := ClientIP(req, true)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestClientIP_UntrustedProxy_IgnoresHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "spoofed-ip")

	// No trusted proxy: ignore X-Forwarded-For even if present
	ip := ClientIP(req, false)
	assert.Equal(t, "192.168.1.100", ip)
}

func TestClientIP_IPv6(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:12345"

	ip := ClientIP(req, false)
	assert.Equal(t, "::1", ip)
}
