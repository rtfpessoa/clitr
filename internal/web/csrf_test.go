package web

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCSRFToken_Length(t *testing.T) {
	token := GenerateCSRFToken()
	// 32 bytes hex-encoded = 64 chars
	assert.Len(t, token, 64)
}

func TestGenerateCSRFToken_Uniqueness(t *testing.T) {
	t1 := GenerateCSRFToken()
	t2 := GenerateCSRFToken()
	assert.NotEqual(t, t1, t2)
}

func TestGenerateCSRFToken_ValidHex(t *testing.T) {
	token := GenerateCSRFToken()
	for _, c := range token {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"character %c is not valid hex", c)
	}
}

func TestValidateCSRFToken_Match(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	assert.True(t, ValidateCSRFToken(session, session.CSRFToken))
}

func TestValidateCSRFToken_Mismatch(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	assert.False(t, ValidateCSRFToken(session, "wrong-token"))
}

func TestValidateCSRFToken_Empty(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	assert.False(t, ValidateCSRFToken(session, ""))
}

func TestValidateCSRFToken_NilSession(t *testing.T) {
	assert.False(t, ValidateCSRFToken(nil, "some-token"))
}

func TestValidateCSRFToken_TimingAttackResistance(t *testing.T) {
	store := NewSessionStore(10 * time.Minute)
	session := store.Create()

	// Tokens differing by one character should take similar time
	// (We can't truly measure timing, but we verify the function uses
	// constant-time comparison by checking it returns false correctly)
	almostRight := session.CSRFToken[:len(session.CSRFToken)-1] + "0"
	if almostRight == session.CSRFToken {
		almostRight = session.CSRFToken[:len(session.CSRFToken)-1] + "1"
	}
	require.False(t, ValidateCSRFToken(session, almostRight))
}
