package ratelimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBurstRefillMemoryAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	l := New(60, 2, 2, func() time.Time { return now })
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("one"); !ok {
			t.Fatal("burst rejected")
		}
	}
	if ok, wait := l.Allow("one"); ok || wait != time.Second {
		t.Fatal("burst not limited")
	}
	now = now.Add(time.Second)
	if ok, _ := l.Allow("one"); !ok {
		t.Fatal("refill failed")
	}
	if ok, _ := l.Allow("two"); !ok {
		t.Fatal("second key rejected")
	}
	if ok, _ := l.Allow("three"); ok || len(l.entries) != 2 {
		t.Fatal("unbounded key state")
	}
	now = now.Add(11 * time.Minute)
	if ok, _ := l.Allow("three"); !ok || len(l.entries) != 1 {
		t.Fatal("idle entries not reclaimed")
	}
}

type observedBody struct{ read bool }

func (b *observedBody) Read([]byte) (int, error) { b.read = true; return 0, io.EOF }
func (*observedBody) Close() error               { return nil }

func TestAdmissionBeforeBodyIgnoresForwardingAndPreservesHealth(t *testing.T) {
	clock := func() time.Time { return time.Unix(1000, 0) }
	login, mcp := New(20, 1, 10, clock), New(120, 1, 10, clock)
	handler := PeerAdmission(login, mcp, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { _, _ = io.ReadAll(r.Body); rw.WriteHeader(200) }))
	for i := 0; i < 2; i++ {
		body := &observedBody{}
		r := httptest.NewRequest("POST", "/mcp", body)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "attacker-"+string(rune('a'+i)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i == 0 && w.Code != 200 {
			t.Fatal("first request rejected")
		}
		if i == 1 && (w.Code != 429 || body.read || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "rate_limited")) {
			t.Fatal("rejected request consumed body or spoofed peer bypassed")
		}
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		r := httptest.NewRequest("GET", path, strings.NewReader(""))
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("health rate limited")
		}
	}
	for i := 0; i < 2; i++ {
		body := &observedBody{}
		r := httptest.NewRequest("GET", "/auth/github/login", body)
		r.RemoteAddr = "127.0.0.1:9876"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i == 0 && w.Code != 200 {
			t.Fatal("login burst rejected")
		}
		if i == 1 && (w.Code != 429 || body.read) {
			t.Fatal("login admission did not reject before body consumption")
		}
	}
}
