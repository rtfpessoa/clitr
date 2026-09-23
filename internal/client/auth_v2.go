package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const (
	webLoginV2Path = "/api/v2/auth/web/login"
	// Trade Republic checks the web frontend version on v2 login requests.
	// Update this when the frontend starts rejecting older versions.
	webAppVersion                 = "2.2631.13"
	webPlatform                   = "web-pro"
	loginWindow                   = 120 * time.Second
	loginPollRate                 = 2 * time.Second
	maxV2ResponseBytes            = 64 * 1024
	deviceIDBytes                 = 64
	millisecondTimestampThreshold = 1e11
	millisecondsPerSecond         = 1000
)

// LoginChallenge describes the second step of a v2 web login.
type LoginChallenge struct {
	Countdown             int
	RequiresAuthenticator bool
}

type loginProcess struct {
	Status         string             `json:"status"`
	RequiredAction string             `json:"requiredAction"`
	ExpiresAt      stdjson.RawMessage `json:"expiresAt"`
}

type loginStartResponse struct {
	ProcessID          string `json:"processId"`
	CountdownInSeconds int    `json:"countdownInSeconds"`
}

type loginCredentials struct {
	phoneNumber string
	pin         string
}

type v2RequestSpec struct {
	method  string
	path    string
	payload any
}

// InitiateWebLoginV2 starts Trade Republic's push approval login.
// The v2 endpoint reaches the application without the WAF token needed by v1.
func (c *Client) InitiateWebLoginV2(phoneNo, pin string) (LoginChallenge, error) {
	var challenge LoginChallenge
	c.processID = ""
	c.v2RequiresAuthenticator = false
	c.v2Deadline = time.Time{}

	result, err := startV2Login(c, loginCredentials{phoneNumber: phoneNo, pin: pin})
	if err != nil {
		return challenge, err
	}
	c.processID = result.ProcessID
	process, err := getV2LoginProcess(c, context.Background())
	if err != nil {
		c.processID = ""
		return challenge, fmt.Errorf("get login approval state: %w", err)
	}
	challenge, c.v2Deadline = challengeFromV2Process(result, process)
	c.v2RequiresAuthenticator = challenge.RequiresAuthenticator
	return challenge, nil
}

func challengeFromV2Process(result loginStartResponse, process loginProcess) (LoginChallenge, time.Time) {
	deadline := time.Now().Add(loginWindow)
	if result.CountdownInSeconds > 0 && result.CountdownInSeconds <= int(loginWindow.Seconds()) {
		deadline = time.Now().Add(time.Duration(result.CountdownInSeconds) * time.Second)
	}
	if expiry, ok := parseLoginExpiry(process.ExpiresAt); ok && expiry.Before(deadline) {
		deadline = expiry
	}
	return LoginChallenge{
		Countdown:             max(1, int(time.Until(deadline).Seconds())),
		RequiresAuthenticator: process.RequiredAction == "AUTHENTICATOR_VERIFICATION",
	}, deadline
}

func startV2Login(c *Client, credentials loginCredentials) (loginStartResponse, error) {
	var result loginStartResponse
	if err := ensureV2DeviceInfo(c); err != nil {
		return result, err
	}
	req, err := newV2Request(c, context.Background(), v2RequestSpec{
		method:  http.MethodPost,
		path:    webLoginV2Path,
		payload: map[string]string{"phoneNumber": credentials.phoneNumber, "pin": credentials.pin},
	})
	if err != nil {
		return result, err
	}
	err = executeV2Request(c, req, &result)
	if err == nil && result.ProcessID == "" {
		err = fmt.Errorf("login response has no process ID")
	}
	return result, err
}

// CompleteWebLoginV2 waits for mobile app approval or submits an authenticator code.
func (c *Client) CompleteWebLoginV2(ctx context.Context, code string) error {
	if err := validateV2Completion(c, code); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, c.v2Deadline)
	defer cancel()

	var err error
	if c.v2RequiresAuthenticator {
		err = submitV2AuthenticatorCode(c, ctx, code)
	} else {
		err = waitForV2Approval(c, ctx)
	}
	if err != nil {
		return err
	}
	if err := c.SaveCookies(); err != nil {
		log.Warn("Failed to save login cookies", zap.Error(err))
	}
	c.processID = ""
	return nil
}

func validateV2Completion(c *Client, code string) error {
	var err error
	switch {
	case c.processID == "":
		err = fmt.Errorf("no process ID available, start a login first")
	case c.v2Deadline.IsZero():
		err = fmt.Errorf("no v2 login is pending")
	case c.v2RequiresAuthenticator && code == "":
		err = fmt.Errorf("authenticator code is required")
	case !c.v2RequiresAuthenticator && code != "":
		err = fmt.Errorf("this login requires approval in the Trade Republic app, not a code")
	}
	return err
}

func waitForV2Approval(c *Client, ctx context.Context) error {
	for {
		process, err := getV2LoginProcess(c, ctx)
		if err != nil {
			return err
		}
		complete, err := checkV2Approval(ctx, process.Status)
		if complete || err != nil {
			return err
		}
	}
}

func checkV2Approval(ctx context.Context, status string) (bool, error) {
	if status == "CONFIRMED" || status == "COMPLETED" || status == "APPROVED" {
		return true, nil
	}
	if status != "PENDING" {
		return false, fmt.Errorf("login approval ended with status %q", status)
	}
	timer := time.NewTimer(loginPollRate)
	defer timer.Stop()
	var err error
	select {
	case <-ctx.Done():
		err = fmt.Errorf("login approval expired: %w", ctx.Err())
	case <-timer.C:
	}
	return false, err
}

func submitV2AuthenticatorCode(c *Client, ctx context.Context, code string) error {
	path := webLoginV2Path + "/processes/" + url.PathEscape(c.processID) + "/authenticator-verification"
	req, err := newV2Request(c, ctx, v2RequestSpec{
		method:  http.MethodPost,
		path:    path,
		payload: map[string]string{"code": code},
	})
	if err != nil {
		return err
	}
	return executeV2Request(c, req, nil)
}

func getV2LoginProcess(c *Client, ctx context.Context) (loginProcess, error) {
	var process loginProcess
	path := webLoginV2Path + "/processes/" + url.PathEscape(c.processID)
	req, err := newV2Request(c, ctx, v2RequestSpec{method: http.MethodGet, path: path})
	if err != nil {
		return process, err
	}
	err = executeV2Request(c, req, &process)
	return process, err
}

func newV2Request(c *Client, ctx context.Context, spec v2RequestSpec) (*http.Request, error) {
	var body io.Reader
	if spec.payload != nil {
		data, err := stdjson.Marshal(spec.payload)
		if err != nil {
			return nil, fmt.Errorf("marshal v2 request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, spec.method, c.apiHost+spec.path, body)
	if err != nil {
		return nil, fmt.Errorf("create v2 request: %w", err)
	}
	setV2Headers(c, req)
	if spec.payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func executeV2Request(c *Client, req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Trade Republic request failed: %w", err)
	}
	defer closeBody(resp.Body)
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxV2ResponseBytes))
	if err == nil {
		err = v2LoginResponseError(resp, data)
	}
	if err == nil && out != nil {
		err = stdjson.Unmarshal(data, out)
	}
	return err
}

func ensureV2DeviceInfo(c *Client) error {
	if c.v2DeviceInfo != "" {
		return nil
	}
	identifier := make([]byte, deviceIDBytes)
	if _, err := rand.Read(identifier); err != nil {
		return fmt.Errorf("generate device ID: %w", err)
	}
	zone := time.Now().Location().String()
	_, offset := time.Now().Zone()
	info := map[string]any{
		"stableDeviceId":     hex.EncodeToString(identifier),
		"browser":            "Chrome",
		"browserVersion":     "146.0.0.0",
		"os":                 runtime.GOOS,
		"timezone":           zone,
		"timezoneOffset":     -offset / int(time.Minute/time.Second),
		"screen":             "1920x1080x24",
		"preferredLanguages": []string{"en"},
		"numberOfCores":      runtime.NumCPU(),
	}
	data, err := stdjson.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal device info: %w", err)
	}
	c.v2DeviceInfo = base64.StdEncoding.EncodeToString(data)
	return nil
}

func setV2Headers(c *Client, req *http.Request) {
	req.Header.Set("User-Agent", UserAgentHeaderValue)
	req.Header.Set("X-TR-Platform", webPlatform)
	req.Header.Set("X-TR-App-Version", webAppVersion)
	req.Header.Set("X-TR-Device-Info", c.v2DeviceInfo)
	req.Header.Set("Accept-Language", "en")
}

func v2LoginResponseError(resp *http.Response, data []byte) error {
	if resp.StatusCode == http.StatusMethodNotAllowed && len(data) == 0 && strings.Contains(strings.ToLower(resp.Header.Get("Server")), "awselb") {
		return fmt.Errorf("Trade Republic's WAF rejected the login request (HTTP 405); try again later")
	}
	var result struct {
		Errors []struct {
			Code string `json:"errorCode"`
		} `json:"errors"`
	}
	if stdjson.Unmarshal(data, &result) == nil && len(result.Errors) > 0 && result.Errors[0].Code != "" {
		return fmt.Errorf("Trade Republic login failed: %s (HTTP %d)", result.Errors[0].Code, resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	return fmt.Errorf("Trade Republic login failed with HTTP %d", resp.StatusCode)
}

func parseLoginExpiry(raw stdjson.RawMessage) (time.Time, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	var timestamp float64
	if stdjson.Unmarshal(raw, &timestamp) == nil {
		if timestamp > millisecondTimestampThreshold {
			timestamp /= millisecondsPerSecond
		}
		return time.Unix(int64(timestamp), 0), true
	}
	var value string
	if stdjson.Unmarshal(raw, &value) == nil {
		when, err := time.Parse(time.RFC3339, value)
		return when, err == nil
	}
	return time.Time{}, false
}
