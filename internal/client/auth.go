package client

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

// UserAgentHeaderValue is the User-Agent header sent to Trade Republic's API.
const UserAgentHeaderValue = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

func (c *Client) collectPin(phoneNumber string) (string, string, error) {
	return collectPinWithReader(bufio.NewReader(c.stdinReader), phoneNumber)
}

func collectPinWithReader(reader *bufio.Reader, phoneNumber string) (string, string, error) {
	if phoneNumber == "" {
		fmt.Print("Phone number (international format, e.g., +4912345678): ")
		phoneNoInput, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read phone number: %w", err)
		}
		phoneNumber = strings.TrimSpace(phoneNoInput)
	}

	fmt.Print("PIN (4 digits): ")
	pinInput, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("failed to read PIN: %w", err)
	}
	return phoneNumber, strings.TrimSpace(pinInput), nil
}

// resumeWebSession attempts to resume a session using saved cookies.
func (c *Client) resumeWebSession() bool {
	if !c.saveCookies {
		return false
	}

	parsedURL, _ := url.Parse(cookiesHost)
	cookies := c.httpClient.Jar.Cookies(parsedURL)
	if len(cookies) == 0 {
		return false
	}

	_, err := c.Settings()
	if err != nil {
		if errors.Is(err, ErrCookiesReset) {
			return false
		}
		log.Error("Failed to get settings", zap.Error(err))
		return false
	}
	return true
}
