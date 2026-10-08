package httpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOperationalRoutes(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		s := New(func(context.Context) error {
			if unavailable {
				return errors.New("secret-sentinel")
			}
			return nil
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		defer s.cancelRequests()
		for _, tc := range []struct {
			method, path string
			status       int
		}{{"GET", "/healthz", 200}, {"POST", "/mcp", 404}, {"GET", "/members", 404}, {"GET", "/readyz", 200}} {
			want := tc.status
			if unavailable && tc.path == "/readyz" {
				want = 503
			}
			w := httptest.NewRecorder()
			s.HTTP.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != want {
				t.Fatalf("%s: got %d want %d", tc.path, w.Code, want)
			}
			if strings.Contains(w.Body.String(), "secret-sentinel") {
				t.Fatal("error leaked")
			}
		}
		s.draining.Store(true)
		w := httptest.NewRecorder()
		s.HTTP.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
		if w.Code != 503 {
			t.Fatal("draining service ready")
		}
	}
}

func TestBodyLimit(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		called := false
		h := bounded(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		r := httptest.NewRequest("POST", "/mcp", bytes.NewReader(bytes.Repeat([]byte("x"), MaxBodyBytes+1)))
		if chunked {
			r.ContentLength = -1
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 413 || called {
			t.Fatal("oversized body processed")
		}
	}
}

func TestReadyDeadline(t *testing.T) {
	s := New(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer s.cancelRequests()
	w := httptest.NewRecorder()
	start := time.Now()
	s.HTTP.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 || time.Since(start) > ReadyTimeout+time.Second {
		t.Fatal("readiness exceeded deadline")
	}
}

func TestSlowHeaders(t *testing.T) {
	s := New(func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, listener) }()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(7 * time.Second))
	start := time.Now()
	_, _ = io.WriteString(conn, "GET /healthz HTTP/1.1\r\nHost: localhost\r\nX-Incomplete: ")
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err == nil && strings.Contains(string(buf[:n]), "200 OK") {
		t.Fatal("incomplete request processed")
	}
	if time.Since(start) > 6*time.Second {
		t.Fatal("header timeout not enforced")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSafeLogs(t *testing.T) {
	var logs bytes.Buffer
	s := New(func(context.Context) error { return errors.New("secret-sentinel") }, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer s.cancelRequests()
	r := httptest.NewRequest("GET", "/readyz?code=secret-sentinel", strings.NewReader("body-sentinel"))
	r.Header.Set("Authorization", "Bearer token-sentinel")
	r.Header.Set("Cookie", "session=cookie-sentinel")
	w := httptest.NewRecorder()
	s.HTTP.Handler.ServeHTTP(w, r)
	for _, s := range []string{"secret-sentinel", "body-sentinel", "token-sentinel", "cookie-sentinel"} {
		if strings.Contains(logs.String()+w.Body.String(), s) {
			t.Fatal("sensitive value exposed")
		}
	}
}
