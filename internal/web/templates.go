package web

import (
	"embed"
	"html/template"
	"io"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

// Templates holds all parsed HTML templates for the web application.
type Templates struct {
	login    *template.Template
	twofa    *template.Template
	progress *template.Template
	result   *template.Template
	errPage  *template.Template
}

// ParseTemplates parses all embedded HTML templates.
// Returns an error if any template fails to parse.
func ParseTemplates() (*Templates, error) {
	base := "templates/base.html"

	login, err := template.ParseFS(templateFS, base, "templates/login.html")
	if err != nil {
		return nil, err
	}

	twofa, err := template.ParseFS(templateFS, base, "templates/twofa.html")
	if err != nil {
		return nil, err
	}

	progress, err := template.ParseFS(templateFS, base, "templates/progress.html")
	if err != nil {
		return nil, err
	}

	result, err := template.ParseFS(templateFS, base, "templates/result.html")
	if err != nil {
		return nil, err
	}

	errPage, err := template.ParseFS(templateFS, base, "templates/error.html")
	if err != nil {
		return nil, err
	}

	return &Templates{
		login:    login,
		twofa:    twofa,
		progress: progress,
		result:   result,
		errPage:  errPage,
	}, nil
}

// LoginData holds the data passed to the login template.
type LoginData struct {
	Nonce     string
	CSRFToken string
	Error     string
}

// TwoFAData holds the data passed to the 2FA template.
type TwoFAData struct {
	Nonce     string
	CSRFToken string
	Countdown int
	Error     string
}

// ProgressData holds the data passed to the progress template.
type ProgressData struct {
	Nonce string
}

// ResultData holds the data passed to the result template.
type ResultData struct {
	Nonce      string
	CSVData    string
	EventCount int
}

// ErrorData holds the data passed to the error template.
type ErrorData struct {
	Nonce string
	Error string
}

// RenderLogin renders the login page.
func (t *Templates) RenderLogin(w io.Writer, data LoginData) error {
	return t.login.Execute(w, data)
}

// RenderTwoFA renders the 2FA verification page.
func (t *Templates) RenderTwoFA(w io.Writer, data TwoFAData) error {
	return t.twofa.Execute(w, data)
}

// RenderProgress renders the progress page with SSE connection.
func (t *Templates) RenderProgress(w io.Writer, data ProgressData) error {
	return t.progress.Execute(w, data)
}

// RenderResult renders the CSV result page.
func (t *Templates) RenderResult(w io.Writer, data ResultData) error {
	return t.result.Execute(w, data)
}

// RenderError renders the error page.
func (t *Templates) RenderError(w http.ResponseWriter, statusCode int, data ErrorData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	// Ignore template render error since we're already writing an error response
	_ = t.errPage.Execute(w, data)
}
