package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// RateLimiter implements a per-IP sliding window rate limiter.
// Each IP address is allowed a maximum number of requests within a time window.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	limit   int
	window  time.Duration
}

// NewRateLimiter creates a rate limiter allowing limit requests per window per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Allow checks whether the given IP is within the rate limit.
// Returns true if the request is allowed, false if rate-limited.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune entries older than the window
	timestamps := rl.entries[ip]
	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}

	if len(pruned) >= rl.limit {
		rl.entries[ip] = pruned
		return false
	}

	rl.entries[ip] = append(pruned, now)
	return true
}

// Cleanup removes entries for IPs with no requests within the window.
// Returns the number of IPs removed.
func (rl *RateLimiter) Cleanup() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)
	var removed int

	for ip, timestamps := range rl.entries {
		// Check if all timestamps are expired
		allExpired := true
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				allExpired = false
				break
			}
		}

		if allExpired {
			delete(rl.entries, ip)
			removed++
		}
	}

	if removed > 0 {
		log.Debug("Rate limiter cleanup", zap.Int("removed", removed))
	}

	return removed
}

// StartCleanup launches a background goroutine that runs Cleanup periodically.
// It stops when the done channel is closed.
func (rl *RateLimiter) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rl.Cleanup()
			case <-done:
				return
			}
		}
	}()
}

// ClientIP extracts the client IP address from a request.
// When trustProxy is true, it reads the leftmost IP from the X-Forwarded-For header.
// When trustProxy is false, it uses RemoteAddr directly, ignoring any forwarding headers.
// This prevents IP spoofing when not behind a trusted reverse proxy.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			// Use the leftmost IP (client IP set by the first proxy)
			ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
