package client

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/zalando/go-keyring"
	"go.uber.org/zap"
)

// savedCookie contains only essential cookie fields to minimize storage size (compact format)
type savedCookie struct {
	Name     string    `json:"n"`
	Value    string    `json:"v"`
	Path     string    `json:"p,omitempty"`
	Domain   string    `json:"d,omitempty"`
	Expires  time.Time `json:"e,omitempty"`
	Secure   bool      `json:"s,omitempty"`
	HttpOnly bool      `json:"h,omitempty"`
	SameSite int       `json:"ss,omitempty"`
}

// ResetCookies removes the saved session from the system keyring.
func (c *Client) ResetCookies() error {
	keyringKey := keyringKeyPfx + c.phoneNo
	if err := keyring.Delete(keyringService, keyringKey); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil // Nothing to delete
		}
		return fmt.Errorf("failed to delete cookies from keyring: %w", err)
	}
	return nil
}

// SaveCookies saves cookies to the system keyring (compressed)
func (c *Client) SaveCookies() error {
	if !c.saveCookies {
		return nil
	}

	parsedURL, err := url.Parse(cookiesHost)
	if err != nil {
		return err
	}
	cookies := c.httpClient.Jar.Cookies(parsedURL)

	if len(cookies) == 0 {
		// No cookies to save - this is normal on first run
		return nil
	}

	encoded, err := encodeCookies(cookies)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringKeyPfx+c.phoneNo, encoded); err != nil {
		return fmt.Errorf("failed to save cookies to keyring: %w", err)
	}
	return nil
}

func encodeCookies(cookies []*http.Cookie) (string, error) {
	var cookieList []savedCookie
	for _, cookie := range cookies {
		cookieList = append(cookieList, savedCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
			SameSite: int(cookie.SameSite),
		})
	}

	jsonData, err := json.Marshal(cookieList)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cookies: %w", err)
	}

	// Compress with gzip to reduce size for keyring storage limits
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	if _, err := gzWriter.Write(jsonData); err != nil {
		return "", fmt.Errorf("failed to compress cookies: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize compression: %w", err)
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

// loadCookies loads cookies from system keyring, with fallback to legacy file for migration
func (c *Client) loadCookies() error {
	keyringKey := keyringKeyPfx + c.phoneNo

	// Try keyring first
	data, err := keyring.Get(keyringService, keyringKey)
	if err == nil {
		// Found in keyring - decompress and parse
		jsonData, decompressErr := decompressCookieData(data)
		if decompressErr != nil {
			log.Debug("failed to decompress keyring data, trying as plain JSON", zap.Error(decompressErr))
			// Try as plain JSON (old format)
			jsonData = []byte(data)
		}
		return c.parseCookiesIntoJar(jsonData)
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		// Keyring error (not just "not found") - log and continue to legacy fallback
		log.Debug("failed to load from keyring, trying legacy file", zap.Error(err))
	}
	return c.loadLegacyCookies()
}

func (c *Client) loadLegacyCookies() error {
	legacyPath := filepath.Join(c.dataDir, "auth", fmt.Sprintf("cookies.%s.json", c.phoneNo))
	fileData, err := readLegacyCookieFile(legacyPath)
	if err != nil {
		return err
	}
	if fileData != nil {
		if err := c.parseCookiesIntoJar(fileData); err != nil {
			return fmt.Errorf("failed to parse legacy cookies: %w", err)
		}
		migrateLegacyCookies(c, legacyPath)
	}
	return nil
}

func readLegacyCookieFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read legacy cookie file: %w", err)
	}
	return data, nil
}

func migrateLegacyCookies(c *Client, legacyPath string) {
	if saveErr := c.SaveCookies(); saveErr != nil {
		log.Warn("failed to migrate cookies to keyring, keeping legacy file", zap.Error(saveErr))
		return
	}

	// Migration successful - delete legacy file
	if removeErr := os.Remove(legacyPath); removeErr != nil {
		log.Warn("failed to remove legacy cookie file after migration", zap.Error(removeErr))
	} else {
		log.Info("migrated cookies from file to system keyring")
	}
}

// decompressCookieData decodes base64 and decompresses gzip data
func decompressCookieData(encoded string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	return decompressed, nil
}

// parseCookiesIntoJar parses cookie JSON data and sets cookies in the HTTP client jar
func (c *Client) parseCookiesIntoJar(data []byte) error {
	var cookieList []savedCookie
	if err := stdjson.Unmarshal(data, &cookieList); err != nil {
		return fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	if len(cookieList) == 0 {
		return nil
	}

	parsedURL, err := url.Parse(cookiesHost)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	var cookies []*http.Cookie
	for _, sc := range cookieList {
		cookies = append(cookies, &http.Cookie{
			Name:     sc.Name,
			Value:    sc.Value,
			Path:     sc.Path,
			Domain:   sc.Domain,
			Expires:  sc.Expires,
			Secure:   sc.Secure,
			HttpOnly: sc.HttpOnly,
			SameSite: http.SameSite(sc.SameSite),
		})
	}

	c.httpClient.Jar.SetCookies(parsedURL, cookies)
	return nil
}
