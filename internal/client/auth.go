package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rtfpessoa/clitr/internal/log"
)

const pinEnvFileName = "pin.env"

// UserAgentHeaderValue is the User-Agent header sent to Trade Republic's API.
const UserAgentHeaderValue = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

// setStdinReader replaces the CLI input source in tests.
func (c *Client) setStdinReader(reader io.Reader) {
	c.stdinReader = reader
}

// ResolvePhoneNumber returns phoneNumber if non-empty, otherwise the PHONE
// value from <dataDir>/pin.env. Callers must resolve the phone number this
// way before constructing a Client so that the key used to load saved
// cookies matches the key later used to save them.
func ResolvePhoneNumber(phoneNumber, dataDir string) string {
	if phoneNumber != "" {
		return phoneNumber
	}
	phone, _ := readCredentialsFromFile(dataDir)
	return phone
}

func (c *Client) collectPin(phoneNumber string) (string, string, error) {
	return collectPinWithReader(bufio.NewReader(c.stdinReader), phoneNumber, c.dataDir)
}

func collectPinWithReader(reader *bufio.Reader, phoneNumber string, dataDir string) (string, string, error) {
	filePhone, filePin := readCredentialsFromFile(dataDir)
	if phoneNumber == "" && filePhone != "" {
		log.Debug("read phone number from pin.env file")
		phoneNumber = filePhone
	}

	if phoneNumber == "" {
		fmt.Print("Phone number (international format, e.g., +4912345678): ")
		phoneNoInput, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read phone number: %w", err)
		}
		phoneNumber = strings.TrimSpace(phoneNoInput)
	}

	if filePin != "" {
		log.Debug("read PIN from pin.env file")
		return phoneNumber, filePin, nil
	}

	fmt.Print("PIN (4 digits): ")
	pinInput, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("failed to read PIN: %w", err)
	}
	return phoneNumber, strings.TrimSpace(pinInput), nil
}

// readCredentialsFromFile reads PHONE and PIN entries from <dataDir>/pin.env
// (e.g. `PIN=1234` / `PHONE=+4912345678`). Missing entries are returned as "",
// so the caller can fall back to prompting the user via stdin.
func readCredentialsFromFile(dataDir string) (phone string, pin string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	expandedDir := dataDir
	if strings.HasPrefix(dataDir, "~/") {
		expandedDir = filepath.Join(home, dataDir[2:])
	}

	data, err := os.ReadFile(filepath.Join(expandedDir, pinEnvFileName))
	if err != nil {
		return "", ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "PIN":
			pin = value
		case "PHONE":
			phone = value
		}
	}
	return phone, pin
}

// resumeWebSession attempts to resume a session using saved cookies.
// The returned error is non-nil only for transient failures (network
// issues, rate limiting, WAF blocks, etc.) where the saved session is
// still presumably valid - callers should surface these instead of
// falling back to a full interactive login, which would both be
// unnecessary and risk compounding server-side rate limiting.
func (c *Client) resumeWebSession() (bool, error) {
	if !c.saveCookies {
		return false, nil
	}

	parsedURL, _ := url.Parse(cookiesHost)
	cookies := c.httpClient.Jar.Cookies(parsedURL)
	if len(cookies) == 0 {
		return false, nil
	}

	_, err := c.Settings()
	if err != nil {
		if errors.Is(err, ErrCookiesReset) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify saved session: %w", err)
	}
	return true, nil
}
