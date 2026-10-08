package mcpclient

import (
	"context"
	"net/http"
	"testing"
)

func TestRejectUnsafeClientConfiguration(t *testing.T) {
	for _, endpoint := range []string{"http://remote.example/mcp", "https://user:secret@example.com/mcp", "https://example.com/mcp?token=secret", "https://example.com/other"} {
		if _, err := Connect(context.Background(), endpoint, "token"); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}

func TestCredentialForwardingRejectsAnotherOrigin(t *testing.T) {
	transport := credentialTransport{token: "synthetic", origin: "https://expected.example", next: http.DefaultTransport}
	req, _ := http.NewRequest("GET", "https://other.example/mcp", nil)
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("credential forwarded")
	}
}
