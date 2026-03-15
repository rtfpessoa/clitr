package web

import (
	"crypto/subtle"
)

// GenerateCSRFToken creates a cryptographically random CSRF token.
// Returns a 64-character hex string (32 random bytes).
func GenerateCSRFToken() string {
	return generateRandomHex(32)
}

// ValidateCSRFToken performs constant-time comparison of the form token
// against the session's stored CSRF token.
// Returns false if the session is nil, the token is empty, or tokens don't match.
func ValidateCSRFToken(session *Session, formToken string) bool {
	if session == nil || formToken == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(session.CSRFToken), []byte(formToken)) == 1
}
