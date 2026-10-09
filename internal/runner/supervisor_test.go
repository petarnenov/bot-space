package runner

import (
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/mcpclient"
)

func TestRetryBackoffByErrorCode(t *testing.T) {
	if got := retryBackoff(&mcpclient.ApplicationError{Code: "rate_limited"}); got != 15*time.Second {
		t.Fatalf("rate_limited backoff = %v", got)
	}
	if got := retryBackoff(&mcpclient.ApplicationError{Code: "temporarily_unavailable"}); got != 5*time.Second {
		t.Fatalf("temporarily_unavailable backoff = %v", got)
	}
	if got := retryBackoff(&mcpclient.ApplicationError{Code: "forbidden"}); got != 2*time.Second {
		t.Fatalf("fallback backoff = %v", got)
	}
}

func TestJitteredBackoffBounds(t *testing.T) {
	base := 4 * time.Second
	for i := 0; i < 100; i++ {
		got := jitteredBackoff(base)
		if got < base || got >= base+base/2 {
			t.Fatalf("jitter out of bounds: %v", got)
		}
	}
}
