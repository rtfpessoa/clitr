package waf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isChromiumAvailable() bool {
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	// macOS Chrome
	if _, err := exec.LookPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
		return true
	}
	return false
}

func TestFetchTokenFromURL_Success(t *testing.T) {
	if !isChromiumAvailable() {
		t.Skip("Chrome/Chromium not available, skipping browser-based test")
	}

	// Local server that sets the aws-waf-token cookie after a short delay via JS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<script>
setTimeout(function() {
	document.cookie = "aws-waf-token=test-token-abc123; path=/; secure";
}, 500);
</script>
</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := FetchTokenFromURL(ctx, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "test-token-abc123", token)
}

func TestFetchTokenFromURL_Timeout(t *testing.T) {
	if !isChromiumAvailable() {
		t.Skip("Chrome/Chromium not available, skipping browser-based test")
	}

	// Server that never sets the cookie
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>No cookie here</body></html>`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := FetchTokenFromURL(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}
