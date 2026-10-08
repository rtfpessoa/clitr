package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression test for a bug where SaveCookies/loadCookies round-tripped
// cookies through a URL with a path component (cookiesHost used to be
// ".../api/v1/auth/web"). http.CookieJar.Cookies() only ever returns a
// cookie's Name/Value, so on reload the jar fell back to RFC 6265's
// default-path algorithm using that URL's path, scoping the reloaded
// cookie to "/api/v1/auth" instead of the "/" the real server sets. That
// made the session cookie silently stop being attached to requests outside
// that path prefix (e.g. /api/v2/auth/account) on every process restart,
// forcing an unnecessary full re-login.
func TestCookies_SurviveRoundTripAcrossProcesses(t *testing.T) {
	tmpDir := t.TempDir()
	phoneNo := "+1234567890"

	c1, err := NewClient(phoneNo, tmpDir, true)
	require.NoError(t, err)
	c1.httpClient.Jar.SetCookies(
		mustParseURL("https://api.traderepublic.com"),
		[]*http.Cookie{{Name: "tr_session", Value: "test-session", Path: "/"}},
	)
	require.NoError(t, c1.SaveCookies())

	c2, err := NewClient(phoneNo, tmpDir, true)
	require.NoError(t, err)

	cookies := c2.httpClient.Jar.Cookies(mustParseURL("https://api.traderepublic.com/api/v2/auth/account"))
	require.Len(t, cookies, 1)
	assert.Equal(t, "tr_session", cookies[0].Name)
	assert.Equal(t, "test-session", cookies[0].Value)
}
