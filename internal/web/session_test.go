package web

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	require.NotNil(t, session)
	assert.NotEmpty(t, session.ID)
	assert.NotEmpty(t, session.CSRFToken)
	assert.Equal(t, StateLogin, session.State)

	got := store.Get(session.ID)
	require.NotNil(t, got)
	assert.Equal(t, session.ID, got.ID)
}

func TestSessionStore_Get_NonExistent(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	got := store.Get("nonexistent-id")
	assert.Nil(t, got)
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	store.Delete(session.ID)

	got := store.Get(session.ID)
	assert.Nil(t, got)
}

func TestSessionStore_Delete_ZerosCSVData(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	session.CSVData = "sensitive,csv,data"

	store.Delete(session.ID)

	// After deletion, the session's CSVData should be zeroed
	assert.Empty(t, session.CSVData)
}

func TestSessionStore_Touch_ExtendsExpiry(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	originalExpiry := session.ExpiresAt

	// Advance time conceptually by touching
	time.Sleep(10 * time.Millisecond)
	store.Touch(session.ID)

	got := store.Get(session.ID)
	require.NotNil(t, got)
	assert.True(t, got.ExpiresAt.After(originalExpiry))
	assert.True(t, got.LastActivity.After(session.CreatedAt))
}

func TestSessionStore_Expire_AfterTTL(t *testing.T) {
	// Use a very short TTL
	store := NewSessionStore(1 * time.Millisecond)

	session := store.Create()
	time.Sleep(5 * time.Millisecond)

	// Get should return nil for expired sessions
	got := store.Get(session.ID)
	assert.Nil(t, got)
}

func TestSessionStore_Cleanup_RemovesExpired(t *testing.T) {
	store := NewSessionStore(1 * time.Millisecond)

	session := store.Create()
	session.CSVData = "to-be-cleaned"

	time.Sleep(5 * time.Millisecond)

	removed := store.Cleanup()
	assert.Equal(t, 1, removed)

	got := store.Get(session.ID)
	assert.Nil(t, got)
}

func TestSessionStore_Cleanup_KeepsActive(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()

	removed := store.Cleanup()
	assert.Equal(t, 0, removed)

	got := store.Get(session.ID)
	require.NotNil(t, got)
	assert.Equal(t, session.ID, got.ID)
}

func TestSession_ID_Is64HexChars(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	// 32 bytes hex-encoded = 64 chars
	assert.Len(t, session.ID, 64)

	// Verify it's valid hex
	for _, c := range session.ID {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"character %c is not valid hex", c)
	}
}

func TestSession_CSRFToken_Is64HexChars(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	session := store.Create()
	assert.Len(t, session.CSRFToken, 64)
}

func TestSession_CSRFToken_Unique(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	s1 := store.Create()
	s2 := store.Create()
	assert.NotEqual(t, s1.CSRFToken, s2.CSRFToken)
}

func TestSessionStore_RotateSession(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	old := store.Create()
	old.State = StateTwoFA
	old.Countdown = 60

	newSession := store.Rotate(old.ID)
	require.NotNil(t, newSession)

	// New session has different ID
	assert.NotEqual(t, old.ID, newSession.ID)
	// Old session is deleted
	assert.Nil(t, store.Get(old.ID))
	// New session preserves state
	assert.Equal(t, StateTwoFA, newSession.State)
	assert.Equal(t, 60, newSession.Countdown)
	// New session has a new CSRF token
	assert.NotEqual(t, old.CSRFToken, newSession.CSRFToken)
}

func TestSessionStore_Rotate_NonExistent(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	result := store.Rotate("nonexistent")
	assert.Nil(t, result)
}

func TestSessionStore_Count(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)

	assert.Equal(t, 0, store.Count())

	store.Create()
	assert.Equal(t, 1, store.Count())

	store.Create()
	assert.Equal(t, 2, store.Count())
}
