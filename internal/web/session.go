// Package web provides the HTTP server infrastructure for the Trade Republic
// transaction CSV webapp, including session management, CSRF protection,
// rate limiting, security middleware, and HTML templates.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// SessionState represents the current step in the authentication flow.
type SessionState string

const (
	StateLogin    SessionState = "login"
	StateTwoFA    SessionState = "twofa"
	StateFetching SessionState = "fetching"
	StateDone     SessionState = "done"
)

// Session holds all per-user state for a single authentication flow.
// All sensitive data lives exclusively in memory and is zeroed on cleanup.
type Session struct {
	ID                    string
	CSRFToken             string
	State                 SessionState
	Countdown             int
	RequiresAuthenticator bool
	CSVData               string
	EventCount            int
	Client                io.Closer // TR client; closed on session delete
	CreatedAt             time.Time
	LastActivity          time.Time
	ExpiresAt             time.Time
}

// SessionStore is a concurrent-safe in-memory session store with TTL-based expiry.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewSessionStore creates a new session store with the given TTL for session expiry.
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// Create generates a new session with a cryptographically random ID and CSRF token.
func (s *SessionStore) Create() *Session {
	now := time.Now()
	session := &Session{
		ID:           generateRandomHex(32),
		CSRFToken:    generateRandomHex(32),
		State:        StateLogin,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(s.ttl),
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	return session
}

// Get retrieves a session by ID. Returns nil if not found or expired.
// Expired sessions are removed on access.
func (s *SessionStore) Get(id string) *Session {
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(session.ExpiresAt) {
		s.Delete(id)
		return nil
	}

	return session
}

// Delete removes a session, closes its client, and zeros its sensitive data.
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	session, ok := s.sessions[id]
	if ok {
		// Close the TR client if present
		if session.Client != nil {
			_ = session.Client.Close()
			session.Client = nil
		}
		// Zero sensitive data before removing reference
		session.CSVData = ""
		session.CSRFToken = ""
		session.EventCount = 0
		delete(s.sessions, id)
	}
	s.mu.Unlock()
}

// Touch updates a session's last activity time and extends its expiry.
func (s *SessionStore) Touch(id string) {
	s.mu.Lock()
	session, ok := s.sessions[id]
	if ok {
		now := time.Now()
		session.LastActivity = now
		session.ExpiresAt = now.Add(s.ttl)
	}
	s.mu.Unlock()
}

// Rotate creates a new session with the same state as the old one but a new ID
// and CSRF token. The old session is deleted. Returns nil if old session not found.
// This defends against session fixation attacks.
func (s *SessionStore) Rotate(oldID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.sessions[oldID]
	if !ok {
		return nil
	}

	now := time.Now()
	newSession := &Session{
		ID:                    generateRandomHex(32),
		CSRFToken:             generateRandomHex(32),
		State:                 old.State,
		Countdown:             old.Countdown,
		RequiresAuthenticator: old.RequiresAuthenticator,
		CSVData:               old.CSVData,
		EventCount:            old.EventCount,
		Client:                old.Client, // Transfer client to new session
		CreatedAt:             now,
		LastActivity:          now,
		ExpiresAt:             now.Add(s.ttl),
	}

	// Zero old session (don't close client — it moved to newSession)
	old.Client = nil
	old.CSVData = ""
	old.CSRFToken = ""
	old.EventCount = 0
	delete(s.sessions, oldID)

	// Store new session
	s.sessions[newSession.ID] = newSession

	return newSession
}

// Cleanup removes all expired sessions and zeros their data.
// Returns the number of sessions removed.
func (s *SessionStore) Cleanup() int {
	now := time.Now()
	var removed int

	s.mu.Lock()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			if session.Client != nil {
				_ = session.Client.Close()
				session.Client = nil
			}
			session.CSVData = ""
			session.CSRFToken = ""
			session.EventCount = 0
			delete(s.sessions, id)
			removed++
		}
	}
	s.mu.Unlock()

	if removed > 0 {
		log.Debug("Session cleanup", zap.Int("removed", removed))
	}

	return removed
}

// Count returns the number of active sessions.
func (s *SessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// StartCleanup launches a background goroutine that runs Cleanup every interval.
// It stops when the done channel is closed.
func (s *SessionStore) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.Cleanup()
			case <-done:
				return
			}
		}
	}()
}

// generateRandomHex generates n random bytes and returns them hex-encoded.
func generateRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read should never fail on supported platforms
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
