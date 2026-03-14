package client

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// UserAgentHeaderValue is the User-Agent header sent to Trade Republic's API.
// It mimics a Chrome browser on macOS to ensure compatibility with their web endpoints.
const UserAgentHeaderValue = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"

func (c *Client) collectPin(phoneNumber string) (*string, error) {
	reader := bufio.NewReader(c.stdinReader)

	// Prompt for phone if not provided
	if phoneNumber == "" {
		fmt.Print("Phone number (international format, e.g., +4912345678): ")
		phoneNoInput, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read phone number: %w", err)
		}
		phoneNumber = strings.TrimSpace(phoneNoInput)
	}

	// Prompt for PIN
	fmt.Print("PIN (4 digits): ")
	pinInput, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read PIN: %w", err)
	}
	pin := strings.TrimSpace(pinInput)

	return &pin, nil
}

// InitiateWebLogin starts the web login process
func (c *Client) initiateWebLogin(phoneNo string) (int, error) {
	pin, err := c.collectPin(phoneNo)
	if err != nil {
		return 0, err
	}

	payload := map[string]string{
		"phoneNumber": phoneNo,
		"pin":         *pin,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal login payload: %w", err)
	}
	req, err := http.NewRequest("POST", c.apiHost+"/api/v1/auth/web/login", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentHeaderValue)

	// Use the client's HTTP client (which has the cookie jar)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to initiate web login: %w", err)
	}
	defer closeBody(resp.Body)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("web login failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ProcessID          string `json:"processId"`
		CountdownInSeconds int    `json:"countdownInSeconds"`
		SecondFactor       string `json:"2fa"`
		Errors             []struct {
			ErrorCode string `json:"errorCode"`
			ErrorMsg  string `json:"errorMsg"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("failed to parse login response: %w", err)
	}

	if len(result.Errors) > 0 {
		return 0, fmt.Errorf("login error: %s - %s", result.Errors[0].ErrorCode, result.Errors[0].ErrorMsg)
	}

	if result.ProcessID == "" {
		return 0, fmt.Errorf("processId not in response")
	}

	c.processID = result.ProcessID
	return result.CountdownInSeconds + 1, nil
}

// CompleteWebLogin completes the web login with 2FA code
func (c *Client) completeWebLogin(code string) error {
	if c.processID == "" {
		return fmt.Errorf("no process ID available, call InitiateWebLogin first")
	}

	webLoginUrl := fmt.Sprintf("%s/api/v1/auth/web/login/%s/%s", c.apiHost, c.processID, code)
	req, err := http.NewRequest("POST", webLoginUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentHeaderValue)

	// Use the client's HTTP client (which has the cookie jar) - this is critical!
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to complete web login: %w", err)
	}
	defer closeBody(resp.Body)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read verification response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("web login verification failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Save cookies after successful login
	if err := c.SaveCookies(); err != nil {
		// Log error but don't fail the login
		log.Warn("Warning: failed to save cookies", zap.Error(err))
	}

	return nil
}

// ResendWebLogin resends the 2FA code
func (c *Client) ResendWebLogin() error {
	if c.processID == "" {
		return fmt.Errorf("no process ID available")
	}

	webLoginCodeResentUrl := fmt.Sprintf("%s/api/v1/auth/web/login/%s/resend", c.apiHost, c.processID)
	req, err := http.NewRequest("POST", webLoginCodeResentUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentHeaderValue)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to resend code: %w", err)
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resend failed with status %d", resp.StatusCode)
	}

	return nil
}

// ResumeWebSession attempts to resume a session using saved cookies
func (c *Client) resumeWebSession() bool {
	if !c.saveCookies {
		return false
	}

	// Check if we have any cookies loaded
	parsedURL, _ := url.Parse(cookiesHost)
	cookies := c.httpClient.Jar.Cookies(parsedURL)
	if len(cookies) == 0 {
		return false
	}

	// Try to fetch settings to validate the session
	_, err := c.Settings()
	if err != nil {
		if errors.Is(err, ErrCookiesReset) {
			return false
		}

		log.Error("Failed to get settings", zap.Error(err))
		return false
	}

	// Session is valid
	return true
}
