package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rtfpessoa/clitr/internal/export"
	"github.com/rtfpessoa/clitr/internal/fetch"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

type sseStream struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
}

func newSSEStream(w http.ResponseWriter, flusher http.Flusher) *sseStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()
	return &sseStream{writer: w, flusher: flusher}
}

func (s *sseStream) write(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.writer, format, args...)
	s.flusher.Flush()
}

func (s *sseStream) startHeartbeat(ctx context.Context) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.write(": keepalive\n\n")
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

// HandleProgressSSE streams transaction fetch progress and the completion event.
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

	stream := newSSEStream(w, flusher)
	defer stream.startHeartbeat(r.Context())()
	result, failure := h.fetchProgressCSV(r.Context(), session, wc, stream)
	if failure != nil {
		log.Error(failure.stage, zap.Error(failure.cause))
		stream.write("event: error_event\ndata: %s\n\n", failure.message)
		return
	}
	h.completeProgressFetch(session, result, stream)
}

type csvResult struct {
	data       string
	eventCount int
}

type progressFailure struct {
	stage   string
	message string
	cause   error
}

func (h *Handlers) fetchProgressCSV(ctx context.Context, session *Session, wc WebClient, stream *sseStream) (csvResult, *progressFailure) {
	rawMaps, err := fetch.FetchAllEvents(ctx, wc, fetch.DirectionAfter, nil, func(page, eventsSoFar int) {
		stream.write("event: progress\ndata: {\"page\":%d,\"events\":%d}\n\n", page, eventsSoFar)
		h.store.Touch(session.ID)
	})
	if err != nil {
		return csvResult{}, &progressFailure{"Fetch failed", "Failed to fetch transactions. Please try again.", err}
	}
	return buildProgressCSV(rawMaps)
}

func buildProgressCSV(rawMaps []map[string]interface{}) (csvResult, *progressFailure) {
	rawEvents, err := fetch.ParseRawMaps(rawMaps)
	if err != nil {
		return csvResult{}, &progressFailure{"Parse raw maps failed", "Failed to parse transactions.", err}
	}
	events, err := export.ParseRawEvents(rawEvents)
	if err != nil {
		return csvResult{}, &progressFailure{"Parse failed", "Failed to parse transactions.", err}
	}
	var csvBuf bytes.Buffer
	if err := export.NewCSVExporter(&csvBuf).Export(events, true); err != nil {
		return csvResult{}, &progressFailure{"CSV export failed", "Failed to generate CSV.", err}
	}
	return csvResult{data: csvBuf.String(), eventCount: len(events)}, nil
}

func (h *Handlers) completeProgressFetch(session *Session, result csvResult, stream *sseStream) {
	session.CSVData = result.data
	session.EventCount = result.eventCount
	session.State = StateDone
	h.store.Touch(session.ID)
	log.Info("Fetch complete", zap.Int("events", result.eventCount))
	stream.write("event: done\ndata: ok\n\n")
}
