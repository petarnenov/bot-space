package integration

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petarnenov/bot-space/internal/mcpserver"
)

func TestMCPDirectAndProxyHostProtection(t *testing.T) {
	f := mailSetup(t)
	handler := mcpserver.New(f.store, nil)
	for _, tc := range []struct {
		name, local, host string
		denied            bool
	}{
		{"direct localhost", "127.0.0.1", "localhost:8080", false},
		{"IPv6 localhost", "::1", "[::1]:8080", false},
		{"rebound localhost", "127.0.0.1", "attacker.example", true},
		{"loopback proxy with public host", "127.0.0.1", "service.example", true},
		{"nonloopback proxy with public host", "10.0.0.2", "service.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/mcp", nil)
			req.Host = tc.host
			req.Header.Set("Authorization", "Bearer "+f.ta)
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(tc.local), Port: 8080}))
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, req)
			if (rw.Code == 403) != tc.denied {
				t.Fatalf("Host protection returned %d", rw.Code)
			}
			if !tc.denied && rw.Code != 405 {
				t.Fatalf("expected stateless SDK to decline GET after authorization, got %d", rw.Code)
			}
		})
	}
}
