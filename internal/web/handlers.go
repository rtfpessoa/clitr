package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/export"
	"github.com/rtfpessoa/clitr/internal/fetch"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

var (
	phoneRegex = regexp.MustCompile(`^\+\d{7,15}$`)
	pinRegex   = regexp.MustCompile(`^\d{4}$`)
	codeRegex  = regexp.MustCompile(`^\d{4}$`)
)

// WebClient defines the interface that handlers need from the Trade Republic client.
// *client.Client satisfies this interface.
type WebClient interface {
	InitiateWebLoginWithCredentials(phone, pin string) (int, error)
	CompleteWebLogin(code string) error
	TimelineTransactions(ctx context.Context, after *string) (string, error)
	TimelineDetailV2(ctx context.Context, timelineID string) (string, error)
	Unsubscribe(ctx context.Context, subscriptionID string) error
	Recv() <-chan client.Message
	Close() error
}

// ClientFactory creates a new WebClient for a given phone number.
// In production, this wraps client.NewClient. In tests, it returns mocks.
type ClientFactory func(phone string) (WebClient, error)

// Handlers holds all HTTP handler methods and their shared dependencies.
type Handlers struct {
	store         *SessionStore
	templates     *Templates
	clientFactory ClientFactory
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(store *SessionStore, templates *Templates, factory ClientFactory) *Handlers {
	return &Handlers{
		store:         store,
		templates:     templates,
		clientFactory: factory,
	}
}

// HandleLogin renders the login form (GET /login).
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	session := h.getOrCreateSession(w, r)
	nonce := NonceFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderLogin(w, LoginData{
		Nonce:     nonce,
		CSRFToken: session.CSRFToken,
	}); err != nil {
		log.Error("Failed to render login template", zap.Error(err))
	}
}

// HandleLoginSubmit processes the login form (POST /login).
// NEVER logs phone or PIN values — only generic status messages.
func (h *Handlers) HandleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	nonce := NonceFromContext(r.Context())

	phone := r.FormValue("phone")
	pin := r.FormValue("pin")

	// Validate phone format
	if !validatePhone(phone) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderLogin(w, LoginData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Error:     "Invalid phone number format. Use international format (e.g., +49123456789).",
		})
		return
	}

	// Validate PIN format
	if !validatePIN(pin) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderLogin(w, LoginData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Error:     "Invalid PIN format. Must be exactly 4 digits.",
		})
		return
	}

	// Create TR client and initiate login
	trClient, err := h.clientFactory(phone)
	if err != nil {
		log.Error("Failed to create client", zap.Error(err))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderLogin(w, LoginData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Error:     "Failed to connect to Trade Republic. Please try again.",
		})
		return
	}

	countdown, err := trClient.InitiateWebLoginWithCredentials(phone, pin)
	if err != nil {
		log.Info("Login initiation failed")
		_ = trClient.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderLogin(w, LoginData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Error:     "Login failed. Please check your credentials and try again.",
		})
		return
	}

	// Store client and update session state
	session.Client = trClient
	session.Countdown = countdown
	session.State = StateTwoFA
	h.store.Touch(session.ID)

	log.Info("Login initiated successfully")
	http.Redirect(w, r, "/twofa", http.StatusSeeOther)
}

// HandleTwoFA renders the 2FA verification form (GET /twofa).
func (h *Handlers) HandleTwoFA(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil || session.State != StateTwoFA {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.store.Touch(session.ID)
	nonce := NonceFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderTwoFA(w, TwoFAData{
		Nonce:     nonce,
		CSRFToken: session.CSRFToken,
		Countdown: session.Countdown,
	}); err != nil {
		log.Error("Failed to render 2FA template", zap.Error(err))
	}
}

// HandleTwoFASubmit processes the 2FA code (POST /twofa).
// NEVER logs the verification code — only generic status messages.
func (h *Handlers) HandleTwoFASubmit(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil || session.State != StateTwoFA {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	nonce := NonceFromContext(r.Context())
	code := r.FormValue("code")

	// Validate code format
	if !validateCode(code) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderTwoFA(w, TwoFAData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Countdown: session.Countdown,
			Error:     "Invalid verification code. Must be exactly 4 digits.",
		})
		return
	}

	// Get the WebClient from the session
	wc, ok := session.Client.(WebClient)
	if !ok || wc == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := wc.CompleteWebLogin(code); err != nil {
		log.Info("2FA verification failed")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.RenderTwoFA(w, TwoFAData{
			Nonce:     nonce,
			CSRFToken: session.CSRFToken,
			Countdown: 0, // Countdown may have expired
			Error:     "Verification failed. Please check your code and try again.",
		})
		return
	}

	// Finding 5: Rotate session after successful 2FA to prevent session fixation
	session.State = StateFetching
	newSession := h.store.Rotate(session.ID)
	if newSession == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Set new session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    newSession.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	log.Info("2FA verification successful, session rotated")
	http.Redirect(w, r, "/progress", http.StatusSeeOther)
}

// HandleProgress renders the progress page (GET /progress).
func (h *Handlers) HandleProgress(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil || (session.State != StateFetching && session.State != StateDone) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// If fetch already completed, redirect to result
	if session.State == StateDone {
		http.Redirect(w, r, "/result", http.StatusSeeOther)
		return
	}

	h.store.Touch(session.ID)
	nonce := NonceFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderProgress(w, ProgressData{
		Nonce: nonce,
	}); err != nil {
		log.Error("Failed to render progress template", zap.Error(err))
	}
}

// HandleProgressSSE streams Server-Sent Events during transaction fetching
// (GET /progress/events).
func (h *Handlers) HandleProgressSSE(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil || session.State != StateFetching {
		http.Error(w, "No active fetch session", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	wc, ok := session.Client.(WebClient)
	if !ok || wc == nil {
		http.Error(w, "No client available", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	flusher.Flush()

	ctx := r.Context()

	// Finding 2: Send heartbeats during fetching to prevent timeout
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// SSE comment for keepalive (not an event, browsers ignore it)
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			case <-heartbeatDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	defer close(heartbeatDone)

	// Fetch all events with progress callback
	rawMaps, err := fetch.FetchAllEvents(ctx, wc, fetch.DirectionAfter, nil,
		func(page int, eventsSoFar int) {
			fmt.Fprintf(w, "event: progress\ndata: {\"page\":%d,\"events\":%d}\n\n", page, eventsSoFar)
			flusher.Flush()
			h.store.Touch(session.ID)
		},
	)
	if err != nil {
		log.Error("Fetch failed", zap.Error(err))
		fmt.Fprintf(w, "event: error_event\ndata: Failed to fetch transactions. Please try again.\n\n")
		flusher.Flush()
		return
	}

	// Convert raw maps to typed events for export
	rawEvents, err := fetch.ParseRawMaps(rawMaps)
	if err != nil {
		log.Error("Parse raw maps failed", zap.Error(err))
		fmt.Fprintf(w, "event: error_event\ndata: Failed to parse transactions.\n\n")
		flusher.Flush()
		return
	}

	// Parse raw events into typed events
	events, err := export.ParseRawEvents(rawEvents)
	if err != nil {
		log.Error("Parse failed", zap.Error(err))
		fmt.Fprintf(w, "event: error_event\ndata: Failed to parse transactions.\n\n")
		flusher.Flush()
		return
	}

	// Convert to CSV
	var csvBuf bytes.Buffer
	exporter := export.NewCSVExporter(&csvBuf)
	if err := exporter.Export(events, true); err != nil {
		log.Error("CSV export failed", zap.Error(err))
		fmt.Fprintf(w, "event: error_event\ndata: Failed to generate CSV.\n\n")
		flusher.Flush()
		return
	}

	// Store CSV in session
	session.CSVData = csvBuf.String()
	session.EventCount = len(events)
	session.State = StateDone
	h.store.Touch(session.ID)

	log.Info("Fetch complete", zap.Int("events", len(events)))

	// Send done event
	fmt.Fprintf(w, "event: done\ndata: ok\n\n")
	flusher.Flush()
}

// HandleResult renders the CSV result page (GET /result).
func (h *Handlers) HandleResult(w http.ResponseWriter, r *http.Request) {
	session := h.getSession(r)
	if session == nil || session.State != StateDone {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	nonce := NonceFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderResult(w, ResultData{
		Nonce:      nonce,
		CSVData:    session.CSVData,
		EventCount: session.EventCount,
	}); err != nil {
		log.Error("Failed to render result template", zap.Error(err))
	}

	// Schedule session cleanup after a short delay to allow clipboard copy
	go func() {
		time.Sleep(5 * time.Minute)
		h.store.Delete(session.ID)
		log.Info("Session cleaned up after result delivery")
	}()
}

// --- Helper methods ---

// getSession retrieves the session from the request cookie.
func (h *Handlers) getSession(r *http.Request) *Session {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}
	return h.store.Get(cookie.Value)
}

// getOrCreateSession retrieves or creates a session, setting the cookie.
func (h *Handlers) getOrCreateSession(w http.ResponseWriter, r *http.Request) *Session {
	if session := h.getSession(r); session != nil {
		return session
	}

	session := h.store.Create()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return session
}

// --- Input validation ---

func validatePhone(phone string) bool {
	return phoneRegex.MatchString(phone)
}

func validatePIN(pin string) bool {
	return pinRegex.MatchString(pin)
}

func validateCode(code string) bool {
	return codeRegex.MatchString(code)
}
