// Package httpserver provides bounded operational HTTP serving and graceful shutdown.
package httpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const MaxBodyBytes = 1 << 20
const DrainTimeout = 20 * time.Second
const ReadyTimeout = 2 * time.Second

type Server struct {
	HTTP           *http.Server
	draining       atomic.Bool
	logger         *slog.Logger
	cancelRequests context.CancelFunc
}

func New(ready func(context.Context) error, logger *slog.Logger) *Server {
	requests, cancel := context.WithCancel(context.Background())
	s := &Server{logger: logger, cancelRequests: cancel}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `{"status":"alive"}`)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), ReadyTimeout)
		defer cancel()
		if s.draining.Load() || ready(ctx) != nil {
			writeJSON(w, http.StatusServiceUnavailable, `{"status":"not_ready"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"ready"}`)
	})
	s.HTTP = &http.Server{
		Handler:           bounded(mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
		BaseContext:    func(net.Listener) context.Context { return requests },
		// net/http diagnostic messages can include untrusted input.
		ErrorLog: log.New(io.Discard, "", 0),
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body+"\n")
}

func bounded(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > MaxBodyBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, `{"error":"body_too_large"}`)
			return
		}
		if r.Body != nil {
			body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
			_ = r.Body.Close()
			if len(body) > MaxBodyBytes {
				writeJSON(w, http.StatusRequestEntityTooLarge, `{"error":"body_too_large"}`)
				return
			}
			if err != nil {
				writeJSON(w, http.StatusBadRequest, `{"error":"invalid_body"}`)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	result := make(chan error, 1)
	go func() { result <- s.HTTP.Serve(listener) }()
	select {
	case err := <-result:
		s.cancelRequests()
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP serving failed")
		}
		return nil
	case <-ctx.Done():
		s.draining.Store(true)
		s.logger.Info("shutdown_started")
	}
	drain, cancel := context.WithTimeout(context.Background(), DrainTimeout)
	defer cancel()
	if err := s.HTTP.Shutdown(drain); err != nil {
		s.cancelRequests()
		_ = s.HTTP.Close()
	}
	s.cancelRequests()
	<-result
	s.logger.Info("shutdown_completed")
	return nil
}
