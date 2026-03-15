package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWebClient implements the interfaces needed by handlers for testing.
type mockWebClient struct {
	initiateResult int
	initiateErr    error
	completeErr    error
	closeCalled    bool
	messages       chan client.Message
}

func newMockWebClient() *mockWebClient {
	return &mockWebClient{
		initiateResult: 60,
		messages:       make(chan client.Message, 100),
	}
}

func (m *mockWebClient) InitiateWebLoginWithCredentials(phone, pin string) (int, error) {
	return m.initiateResult, m.initiateErr
}

func (m *mockWebClient) CompleteWebLogin(code string) error {
	return m.completeErr
}

func (m *mockWebClient) TimelineTransactions(ctx context.Context, after *string) (string, error) {
	return "sub-1", nil
}

func (m *mockWebClient) TimelineDetailV2(ctx context.Context, timelineID string) (string, error) {
	return "sub-2", nil
}

func (m *mockWebClient) Unsubscribe(ctx context.Context, subscriptionID string) error {
	return nil
}

func (m *mockWebClient) Recv() <-chan client.Message {
	return m.messages
}

func (m *mockWebClient) Close() error {
	m.closeCalled = true
	return nil
}

// --- Helper to create a Handlers instance for testing ---

func newTestHandlers(t *testing.T) (*Handlers, *SessionStore) {
	t.Helper()
	store := NewSessionStore(10 * time.Minute)
	templates, err := ParseTemplates()
	require.NoError(t, err)

	h := &Handlers{
		store:     store,
		templates: templates,
		clientFactory: func(phone string) (WebClient, error) {
			return newMockWebClient(), nil
		},
	}

	return h, store
}

func newTestHandlersWithMock(t *testing.T, mock *mockWebClient) (*Handlers, *SessionStore) {
	t.Helper()
	store := NewSessionStore(10 * time.Minute)
	templates, err := ParseTemplates()
	require.NoError(t, err)

	h := &Handlers{
		store:     store,
		templates: templates,
		clientFactory: func(phone string) (WebClient, error) {
			return mock, nil
		},
	}

	return h, store
}

// --- Login handler tests ---

func TestHandleLogin_GET_RendersForm(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()

	h.HandleLogin(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `name="phone"`)
	assert.Contains(t, body, `name="pin"`)
	assert.Contains(t, body, `name="csrf_token"`)
	// Should have set a session cookie
	cookies := rec.Result().Cookies()
	assert.NotEmpty(t, cookies)
	found := false
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = true
			assert.True(t, c.HttpOnly)
			assert.Equal(t, "/", c.Path)
		}
	}
	assert.True(t, found, "session cookie should be set")
}

func TestHandleLoginSubmit_POST_ValidCredentials_RedirectsTo2FA(t *testing.T) {
	mock := newMockWebClient()
	mock.initiateResult = 60
	h, store := newTestHandlersWithMock(t, mock)

	// Create a session first (as the GET handler would)
	session := store.Create()

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("phone", "+49123456789")
	form.Set("pin", "1234")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleLoginSubmit(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/twofa", rec.Header().Get("Location"))
}

func TestHandleLoginSubmit_POST_InvalidPhone_ShowsError(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("phone", "not-a-phone")
	form.Set("pin", "1234")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleLoginSubmit(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Invalid phone number")
}

func TestHandleLoginSubmit_POST_InvalidPIN_ShowsError(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("phone", "+49123456789")
	form.Set("pin", "abc")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleLoginSubmit(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Invalid PIN")
}

func TestHandleLoginSubmit_POST_NoSession_RedirectsToLogin(t *testing.T) {
	h, _ := newTestHandlers(t)

	form := url.Values{}
	form.Set("phone", "+49123456789")
	form.Set("pin", "1234")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.HandleLoginSubmit(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestHandleLoginSubmit_POST_AuthError_ShowsError(t *testing.T) {
	mock := newMockWebClient()
	mock.initiateErr = fmt.Errorf("invalid credentials")
	h, store := newTestHandlersWithMock(t, mock)
	session := store.Create()

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("phone", "+49123456789")
	form.Set("pin", "1234")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleLoginSubmit(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Login failed")
}

// --- TwoFA handler tests ---

func TestHandleTwoFA_GET_RendersForm(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()
	session.State = StateTwoFA
	session.Countdown = 60

	req := httptest.NewRequest("GET", "/twofa", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleTwoFA(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `name="code"`)
	assert.Contains(t, body, "60")
}

func TestHandleTwoFA_GET_NoSession_RedirectsToLogin(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/twofa", nil)
	rec := httptest.NewRecorder()

	h.HandleTwoFA(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestHandleTwoFASubmit_POST_ValidCode_RedirectsToProgress(t *testing.T) {
	mock := newMockWebClient()
	h, store := newTestHandlersWithMock(t, mock)
	session := store.Create()
	session.State = StateTwoFA
	session.Client = mock

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("code", "1234")

	req := httptest.NewRequest("POST", "/twofa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleTwoFASubmit(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/progress", rec.Header().Get("Location"))

	// Session should have been rotated (Finding 5: session fixation)
	assert.Nil(t, store.Get(session.ID), "old session should be deleted after rotation")
	// A new session cookie should be set
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = true
			assert.NotEqual(t, session.ID, c.Value, "new session ID should differ")
		}
	}
	assert.True(t, found, "new session cookie should be set")
}

func TestHandleTwoFASubmit_POST_InvalidCode_ShowsError(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()
	session.State = StateTwoFA

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("code", "abc")

	req := httptest.NewRequest("POST", "/twofa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleTwoFASubmit(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Invalid verification code")
}

func TestHandleTwoFASubmit_POST_AuthError_ShowsError(t *testing.T) {
	mock := newMockWebClient()
	mock.completeErr = fmt.Errorf("wrong code")
	h, store := newTestHandlersWithMock(t, mock)
	session := store.Create()
	session.State = StateTwoFA
	session.Client = mock

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	form.Set("code", "9999")

	req := httptest.NewRequest("POST", "/twofa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleTwoFASubmit(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Verification failed")
}

// --- Progress handler tests ---

func TestHandleProgress_GET_RendersPage(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()
	session.State = StateFetching

	req := httptest.NewRequest("GET", "/progress", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleProgress(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "EventSource")
	assert.Contains(t, body, "progress-bar")
}

func TestHandleProgress_GET_NoSession_RedirectsToLogin(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/progress", nil)
	rec := httptest.NewRecorder()

	h.HandleProgress(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

// --- Result handler tests ---

func TestHandleResult_GET_RendersCSV(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()
	session.State = StateDone
	session.CSVData = "date;type;amount\n2024-01-15;buy;100.00"
	session.EventCount = 1

	req := httptest.NewRequest("GET", "/result", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleResult(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "date;type;amount")
	assert.Contains(t, body, "Copy to Clipboard")
	assert.Contains(t, body, "1 transactions exported")
}

func TestHandleResult_GET_NoSession_RedirectsToLogin(t *testing.T) {
	h, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/result", nil)
	rec := httptest.NewRecorder()

	h.HandleResult(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestHandleResult_GET_WrongState_RedirectsToLogin(t *testing.T) {
	h, store := newTestHandlers(t)
	session := store.Create()
	session.State = StateLogin // Wrong state

	req := httptest.NewRequest("GET", "/result", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	h.HandleResult(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

// --- Input validation tests ---

func TestValidatePhone(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"+49123456789", true},
		{"+1234567", true},       // Minimum length
		{"+123456789012345", true}, // Maximum length
		{"", false},
		{"49123456789", false},    // Missing +
		{"+123", false},           // Too short
		{"+1234567890123456", false}, // Too long
		{"+49abc", false},         // Letters
		{"+49 123 456", false},    // Spaces
		{"' OR 1=1 --", false},    // SQL injection
		{"<script>alert(1)</script>", false}, // XSS attempt
		{"+49123456789\n", false}, // Newline injection
		{"+49123456789\x00", false}, // Null byte
	}

	for _, tc := range tests {
		t.Run(tc.phone, func(t *testing.T) {
			assert.Equal(t, tc.valid, validatePhone(tc.phone), "phone: %q", tc.phone)
		})
	}
}

func TestValidatePIN(t *testing.T) {
	tests := []struct {
		pin   string
		valid bool
	}{
		{"1234", true},
		{"0000", true},
		{"", false},
		{"123", false},  // Too short
		{"12345", false}, // Too long
		{"abcd", false},  // Letters
		{"12 4", false},  // Space
		{"1234; DROP TABLE users;--", false}, // SQL injection
		{"<script>", false},                  // XSS attempt
		{"12\n4", false},                     // Newline injection
	}

	for _, tc := range tests {
		t.Run(tc.pin, func(t *testing.T) {
			assert.Equal(t, tc.valid, validatePIN(tc.pin), "pin: %q", tc.pin)
		})
	}
}

func TestValidateCode(t *testing.T) {
	tests := []struct {
		code  string
		valid bool
	}{
		{"1234", true},
		{"0000", true},
		{"", false},
		{"123", false},
		{"12345", false},
		{"abcd", false},
		{"12;DROP", false}, // SQL injection
		{"<img>", false},   // XSS attempt
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			assert.Equal(t, tc.valid, validateCode(tc.code), "code: %q", tc.code)
		})
	}
}
