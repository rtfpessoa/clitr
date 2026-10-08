package client

import (
	stdjson "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// rateLimitCooldown is how long we wait before attempting another login
// after Trade Republic responds with HTTP 429, to avoid triggering more
// aggressive server-side blocking from repeated attempts.
const rateLimitCooldown = 10 * time.Minute

type rateLimitState struct {
	HitAt time.Time `json:"hitAt"`
}

func rateLimitPath(dataDir string) string {
	return filepath.Join(dataDir, "auth", "ratelimit.json")
}

// recordRateLimit persists the time a 429 was seen, so future runs can skip
// login attempts until the cooldown passes.
func recordRateLimit(dataDir string) error {
	path := rateLimitPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create rate limit dir: %w", err)
	}
	data, err := stdjson.Marshal(rateLimitState{HitAt: time.Now()})
	if err != nil {
		return fmt.Errorf("failed to marshal rate limit state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to save rate limit state: %w", err)
	}
	return nil
}

// rateLimitRemaining returns how much longer the login cooldown lasts, or
// zero if there is no recorded rate limit or the cooldown has passed.
func rateLimitRemaining(dataDir string) time.Duration {
	data, err := os.ReadFile(rateLimitPath(dataDir))
	if err != nil {
		return 0
	}
	var state rateLimitState
	if err := stdjson.Unmarshal(data, &state); err != nil {
		return 0
	}
	remaining := rateLimitCooldown - time.Since(state.HitAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
