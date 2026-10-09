package identity

import (
	"net/http/httptest"
	"testing"

	"github.com/petarnenov/bot-space/internal/config"
)

func TestSameOriginAcceptsMissingBrowserMetadata(t *testing.T) {
	web := &Web{Config: config.Identity{BaseURL: "https://app.example"}}
	request := httptest.NewRequest("POST", "https://app.example/objectives", nil)
	if !web.sameOrigin(request) {
		t.Fatal("browser request rejected without origin metadata")
	}

	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if !web.sameOrigin(request) {
		t.Fatal("same-origin browser request rejected without Referer")
	}

	request.Header.Set("Sec-Fetch-Site", "cross-site")
	if web.sameOrigin(request) {
		t.Fatal("cross-site browser request admitted")
	}

	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if web.sameOrigin(request) {
		t.Fatal("Fetch Metadata overrode a foreign Origin")
	}

	for _, tc := range []struct {
		name   string
		origin []string
		want   originRejection
	}{
		{"opaque", []string{"null"}, originOpaque},
		{"foreign", []string{"https://evil.example"}, originForeign},
		{"duplicate", []string{"https://app.example", "https://app.example"}, originMultiple},
		{"malformed", []string{"https://user@app.example"}, originMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://app.example/objectives", nil)
			for _, value := range tc.origin {
				req.Header.Add("Origin", value)
			}
			if got := web.originRejection(req); got != tc.want {
				t.Fatalf("origin rejection = %q, want %q", got, tc.want)
			}
		})
	}
}
