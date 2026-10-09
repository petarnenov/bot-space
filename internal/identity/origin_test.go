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
}
