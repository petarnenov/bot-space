package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func TestSignedInHomeTemplateIncludesIdentityAndNavigation(t *testing.T) {
	rw := httptest.NewRecorder()
	render(rw, page{Title: "The Firm", Session: &identity.Session{User: workspaces.User{Username: "<member>"}}})
	body := rw.Body.String()
	if !strings.Contains(body, "&lt;member&gt;") || !strings.Contains(body, "No workspace memberships") || !strings.Contains(body, "/profile") {
		t.Fatalf("signed-in home is incomplete: %s", body)
	}
}
