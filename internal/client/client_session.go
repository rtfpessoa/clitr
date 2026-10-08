package client

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const webSessionLifetime = 290 * time.Second

// setAPIHost replaces the API host in tests.
func (c *Client) setAPIHost(host string) {
	c.apiHost = host
}

// RefreshWebSession refreshes the web session token
func (c *Client) refreshWebSession() error {
	if time.Now().Before(c.webSessionTokenExpires) {
		return nil // Token still valid
	}

	req, err := http.NewRequest("GET", c.apiHost+"/api/v1/auth/web/session", nil)
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentHeaderValue)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return sessionRefreshError(c, resp)
	}

	c.webSessionTokenExpires = time.Now().Add(webSessionLifetime)

	// Trade Republic may rotate tr_session/tr_refresh on each refresh. Persist
	// the updated cookie jar so the next invocation doesn't try to reuse
	// stale, already-rotated cookies and get forced into a full re-login.
	if err := c.SaveCookies(); err != nil {
		log.Warn("failed to persist refreshed session cookies", zap.Error(err))
	}

	return nil
}

func sessionRefreshError(c *Client, resp *http.Response) error {
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body", zap.Error(err))
	}
	if strings.Contains(string(bodyText), "AUTHENTICATION_ERROR") {
		// The server has definitively told us the session is invalid, so we
		// must report ErrCookiesReset regardless of whether clearing the
		// stored copy succeeds - that's just best-effort cleanup and its
		// failure (e.g. keyring unavailable) must not be mistaken for a
		// transient error, which would block the fallback to a fresh login.
		if err := c.ResetCookies(); err != nil {
			log.Warn("failed to clear stored cookies after session invalidation", zap.Error(err))
		}
		return ErrCookiesReset
	}
	return fmt.Errorf("session refresh failed with status %d: %s", resp.StatusCode, bodyText)
}

// WebRequest makes an authenticated web request
func (c *Client) webRequest(method, path string) (*http.Response, error) {
	if err := c.refreshWebSession(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.apiHost+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgentHeaderValue)

	return c.httpClient.Do(req)
}

// Settings retrieves account settings
func (c *Client) Settings() (map[string]interface{}, error) {
	resp, err := c.webRequest("GET", "/api/v2/auth/account")
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("settings request failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode settings: %w", err)
	}

	return result, nil
}
