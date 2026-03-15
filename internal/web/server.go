package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const (
	sessionTTL       = 10 * time.Minute
	cleanupInterval  = 60 * time.Second
	rateLimitPerMin  = 5
	rateLimitWindow  = 1 * time.Minute
	readTimeout      = 10 * time.Second
	writeTimeout     = 0 // Disabled for SSE (Finding 2); per-handler ctx timeouts instead
	idleTimeout      = 120 * time.Second
	shutdownTimeout  = 30 * time.Second
)

// ServerConfig holds configuration for the web server.
type ServerConfig struct {
	Host       string
	Port       int
	TrustProxy bool
}

// Server is the main HTTP server for the webapp.
type Server struct {
	httpServer *http.Server
	handler    http.Handler
	store      *SessionStore
	limiter    *RateLimiter
	done       chan struct{}
	addr       string
}

// NewServer creates and configures a new web server.
func NewServer(cfg ServerConfig, factory ClientFactory) (*Server, error) {
	templates, err := ParseTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	store := NewSessionStore(sessionTTL)
	limiter := NewRateLimiter(rateLimitPerMin, rateLimitWindow)
	handlers := NewHandlers(store, templates, factory)
	done := make(chan struct{})

	// Start background cleanup goroutines
	store.StartCleanup(cleanupInterval, done)
	limiter.StartCleanup(cleanupInterval, done)

	// Build route mux
	mux := http.NewServeMux()

	// GET / → redirect to /login
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Login routes
	mux.HandleFunc("GET /login", handlers.HandleLogin)
	mux.HandleFunc("POST /login", handlers.HandleLoginSubmit)

	// 2FA routes
	mux.HandleFunc("GET /twofa", handlers.HandleTwoFA)
	mux.HandleFunc("POST /twofa", handlers.HandleTwoFASubmit)

	// Progress routes
	mux.HandleFunc("GET /progress", handlers.HandleProgress)
	mux.HandleFunc("GET /progress/events", handlers.HandleProgressSSE)

	// Result route
	mux.HandleFunc("GET /result", handlers.HandleResult)

	// Apply middleware chain: logging → security headers → body size limit → rate limit → CSRF → mux
	var handler http.Handler = mux
	handler = CSRFMiddleware(store)(handler)
	handler = RateLimitMiddleware(limiter, cfg.TrustProxy)(handler)
	handler = BodySizeLimit(handler)
	handler = SecurityHeaders(handler)
	handler = RequestLogging(handler)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	srv := &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		},
		handler: handler,
		store:   store,
		limiter: limiter,
		done:    done,
		addr:    addr,
	}

	return srv, nil
}

// Start begins listening and serving HTTP requests.
// It blocks until the server is shut down.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	// Update addr with the actual port (useful when port is 0)
	s.addr = listener.Addr().String()
	log.Info("Web server starting", zap.String("addr", s.addr))

	err = s.httpServer.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string {
	return s.addr
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Info("Shutting down web server")

	// Stop background goroutines
	close(s.done)

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	// Clean up all remaining sessions
	s.store.Cleanup()

	log.Info("Web server shut down")
	return nil
}
