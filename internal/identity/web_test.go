package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petarnenov/bot-space/internal/config"
)

func TestSessionCookieProtections(t *testing.T) {
	w := &Web{Config: config.Identity{SecureCookies: true}}
	rw := httptest.NewRecorder()
	w.setCookie(rw, w.CookieName(), "synthetic-session", int(SessionLifetime.Seconds()))
	cookies := rw.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session cookie")
	}
	c := cookies[0]
	if c.Name != "__Host-"+SessionCookieName || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" || c.MaxAge != 28800 {
		t.Fatal("insecure production session cookie")
	}
	rw = httptest.NewRecorder()
	w.setCookie(rw, w.CookieName(), "", -1)
	if rw.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout cookie not expired")
	}
}
