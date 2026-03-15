package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplates_AllParse(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)
	require.NotNil(t, templates)
	assert.NotNil(t, templates.login)
	assert.NotNil(t, templates.twofa)
	assert.NotNil(t, templates.progress)
	assert.NotNil(t, templates.result)
	assert.NotNil(t, templates.errPage)
}

func TestTemplates_LoginRender(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderLogin(&buf, LoginData{
		Nonce:     "test-nonce-123",
		CSRFToken: "csrf-token-456",
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, `name="phone"`)
	assert.Contains(t, html, `name="pin"`)
	assert.Contains(t, html, `name="csrf_token"`)
	assert.Contains(t, html, "csrf-token-456")
	assert.Contains(t, html, `nonce="test-nonce-123"`)
	assert.Contains(t, html, `type="submit"`)
}

func TestTemplates_LoginRender_WithError(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderLogin(&buf, LoginData{
		Nonce:     "nonce",
		CSRFToken: "csrf",
		Error:     "Invalid phone number format",
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Invalid phone number format")
	assert.Contains(t, html, "error-message")
}

func TestTemplates_LoginRender_NoError(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderLogin(&buf, LoginData{
		Nonce:     "nonce",
		CSRFToken: "csrf",
	})
	require.NoError(t, err)

	html := buf.String()
	// The CSS class "error-message" appears in the stylesheet, but there should be
	// no <div class="error-message"> element rendered when Error is empty
	assert.NotContains(t, html, `<div class="error-message">`)
}

func TestTemplates_TwoFARender(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderTwoFA(&buf, TwoFAData{
		Nonce:     "twofa-nonce",
		CSRFToken: "twofa-csrf",
		Countdown: 60,
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, `name="code"`)
	assert.Contains(t, html, `name="csrf_token"`)
	assert.Contains(t, html, "twofa-csrf")
	assert.Contains(t, html, `nonce="twofa-nonce"`)
	assert.Contains(t, html, "60")
}

func TestTemplates_ProgressRender(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderProgress(&buf, ProgressData{
		Nonce: "progress-nonce",
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "EventSource")
	assert.Contains(t, html, `nonce="progress-nonce"`)
	assert.Contains(t, html, "progress-bar")
}

func TestTemplates_ResultRender(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderResult(&buf, ResultData{
		Nonce:      "result-nonce",
		CSVData:    "date,type,amount\n2024-01-15,buy,100.00",
		EventCount: 42,
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Copy to Clipboard")
	assert.Contains(t, html, `nonce="result-nonce"`)
	assert.Contains(t, html, "date,type,amount")
	assert.Contains(t, html, "42 transactions exported")
	assert.Contains(t, html, "clipboard")
}

func TestTemplates_ErrorRender(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.errPage.Execute(&buf, ErrorData{
		Nonce: "err-nonce",
		Error: "Something went wrong",
	})
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Something went wrong")
	assert.Contains(t, html, "Try Again")
	assert.Contains(t, html, "/login")
}

func TestTemplates_HTMLEscaping(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = templates.RenderLogin(&buf, LoginData{
		Nonce:     "nonce",
		CSRFToken: "csrf",
		Error:     "<script>alert('xss')</script>",
	})
	require.NoError(t, err)

	html := buf.String()
	// html/template auto-escapes HTML
	assert.NotContains(t, html, "<script>alert('xss')</script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

func TestTemplates_AllHaveDoctype(t *testing.T) {
	templates, err := ParseTemplates()
	require.NoError(t, err)

	testCases := []struct {
		name   string
		render func(buf *bytes.Buffer) error
	}{
		{"login", func(buf *bytes.Buffer) error {
			return templates.RenderLogin(buf, LoginData{Nonce: "n", CSRFToken: "c"})
		}},
		{"twofa", func(buf *bytes.Buffer) error {
			return templates.RenderTwoFA(buf, TwoFAData{Nonce: "n", CSRFToken: "c"})
		}},
		{"progress", func(buf *bytes.Buffer) error {
			return templates.RenderProgress(buf, ProgressData{Nonce: "n"})
		}},
		{"result", func(buf *bytes.Buffer) error {
			return templates.RenderResult(buf, ResultData{Nonce: "n"})
		}},
		{"error", func(buf *bytes.Buffer) error {
			return templates.errPage.Execute(buf, ErrorData{Nonce: "n", Error: "e"})
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tc.render(&buf)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(buf.String(), "<!DOCTYPE html>"),
				"template %s should start with DOCTYPE", tc.name)
		})
	}
}
