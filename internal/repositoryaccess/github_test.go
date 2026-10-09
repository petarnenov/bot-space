package repositoryaccess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

var repo = Repository{ID: 42, OwnerID: 1, Owner: "owner", Name: "project"}

const details = `{"id":42,"name":"project","owner":{"id":1,"login":"owner"}}`

func fixture(t *testing.T, collaborators string) (*Checker, *time.Time, *int) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	c, err := New(func(context.Context) (string, error) { return "private-test-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now }
	c.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer private-test-token" {
			t.Fatal("unsafe API routing")
		}
		if r.URL.Path == "/repos/owner/project" {
			return response(200, details), nil
		}
		if r.URL.Query().Get("affiliation") != "direct" || r.URL.Query().Get("per_page") != "100" {
			t.Fatal("not checking direct collaborators")
		}
		return response(200, collaborators), nil
	})
	return c, &now, &calls
}
func TestOwnerCollaboratorAndPublicReader(t *testing.T) {
	c, _, _ := fixture(t, `[{"id":2,"login":"renamed-user"}]`)
	for _, id := range []int64{1, 2} {
		if err := c.Verify(context.Background(), repo, id, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Verify(context.Background(), repo, 3, false); !errors.Is(err, ErrDenied) {
		t.Fatal("public reader admitted", err)
	}
	wrong := repo
	wrong.ID = 43
	if err := c.Verify(context.Background(), wrong, 1, true); !errors.Is(err, ErrUnavailable) {
		t.Fatal("repository replacement admitted", err)
	}
}
func TestCacheBoundRemovalRefreshAndOutage(t *testing.T) {
	c, now, calls := fixture(t, `[{"id":2}]`)
	ctx := context.Background()
	if err := c.Verify(ctx, repo, 2, false); err != nil {
		t.Fatal(err)
	}
	initial := *calls
	*now = now.Add(59 * time.Second)
	if err := c.Verify(ctx, repo, 2, false); err != nil || *calls != initial {
		t.Fatal("cache not reused")
	}
	c.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/project") {
			return response(200, details), nil
		}
		return response(200, `[]`), nil
	})
	*now = now.Add(time.Second)
	if err := c.Verify(ctx, repo, 2, false); !errors.Is(err, ErrDenied) {
		t.Fatal("removal exceeded 60 second bound", err)
	}
	c, _, _ = fixture(t, `[{"id":2}]`)
	if err := c.Verify(ctx, repo, 2, false); err != nil {
		t.Fatal(err)
	}
	c.client.Transport = transport(func(*http.Request) (*http.Response, error) { return response(503, `secret response sentinel`), nil })
	if err := c.Verify(ctx, repo, 2, true); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "sentinel") {
		t.Fatal("refresh did not fail safely", err)
	}
	if err := c.Verify(ctx, repo, 2, false); !errors.Is(err, ErrUnavailable) {
		t.Fatal("stale authority survived failed refresh", err)
	}
}
func TestPaginationAndMalformedProviderFailClosed(t *testing.T) {
	c, _, _ := fixture(t, `[]`)
	c.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/repos/owner/project" {
			return response(200, details), nil
		}
		if r.URL.Query().Get("page") == "2" {
			return response(200, `[{"id":999}]`), nil
		}
		items := make([]string, 100)
		for i := range items {
			items[i] = fmt.Sprintf(`{"id":%d}`, i+2)
		}
		return response(200, "["+strings.Join(items, ",")+"]"), nil
	})
	if err := c.Verify(context.Background(), repo, 999, true); err != nil {
		t.Fatal("second page membership lost", err)
	}
	for _, code := range []int{301, 401, 403, 404, 429, 500} {
		c, _, _ = fixture(t, `[]`)
		c.client.Transport = transport(func(*http.Request) (*http.Response, error) { return response(code, details), nil })
		if err := c.Verify(context.Background(), repo, 1, true); !errors.Is(err, ErrUnavailable) {
			t.Fatal("provider failure admitted", code, err)
		}
	}
	c, _, _ = fixture(t, `[]`)
	c.client.Transport = transport(func(*http.Request) (*http.Response, error) {
		return response(200, strings.Repeat("x", BodyLimit+1)), nil
	})
	if err := c.Verify(context.Background(), repo, 1, true); !errors.Is(err, ErrUnavailable) {
		t.Fatal("oversized response admitted", err)
	}
}
func TestConcurrentOldVerificationCannotRestoreRemovedAccess(t *testing.T) {
	c, _, _ := fixture(t, `[]`)
	c.now = time.Now
	blocked := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	collaboratorCalls := 0
	c.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/repos/owner/project" {
			return response(200, details), nil
		}
		mu.Lock()
		collaboratorCalls++
		n := collaboratorCalls
		mu.Unlock()
		if n == 1 {
			close(blocked)
			<-release
			return response(200, `[{"id":2}]`), nil
		}
		return response(200, `[]`), nil
	})
	done := make(chan error, 1)
	go func() { done <- c.Verify(context.Background(), repo, 2, true) }()
	<-blocked
	if err := c.Verify(context.Background(), repo, 2, true); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrUnavailable) {
		t.Fatal("old in-flight check granted access", err)
	}
	if err := c.Verify(context.Background(), repo, 2, false); !errors.Is(err, ErrDenied) {
		t.Fatal("removed authority resurrected", err)
	}
}
