package identity

import (
	"context"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

const SessionCookieName = "bot_space_session"
const AttemptCookieName = "bot_space_oauth_state"

type Routes interface{ Handle(string, http.Handler) }
type Web struct {
	Config            config.Identity
	Sessions          *Sessions
	Workspaces        *workspaces.Store
	Provider          Provider
	HomeRenderer      func(http.ResponseWriter, *http.Request, *Session)
	WorkspaceRenderer func(http.ResponseWriter, *http.Request, Session)
	AfterGitHubLogin  func(context.Context, workspaces.User, string) error
}

func (w *Web) CookieName() string {
	if w.Config.SecureCookies {
		return "__Host-" + SessionCookieName
	}
	return SessionCookieName
}
func (w *Web) attemptName() string {
	if w.Config.SecureCookies {
		return "__Host-" + AttemptCookieName
	}
	return AttemptCookieName
}

func (w *Web) Register(routes Routes) {
	routes.Handle("GET /{$}", w.headers(http.HandlerFunc(w.home)))
	routes.Handle("GET /auth/github/login", w.headers(http.HandlerFunc(w.login)))
	routes.Handle("GET /auth/github/callback", w.headers(http.HandlerFunc(w.callback)))
	routes.Handle("POST /auth/logout", w.Protect(true, w.logout))
	routes.Handle("GET /workspaces/{workspaceID}", w.Protect(false, w.workspace))
	routes.Handle("POST /invitations/accept", w.Protect(true, w.acceptInvitation))
}

func (w *Web) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "no-store")
		rw.Header().Set("Referrer-Policy", "no-referrer")
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(rw, r)
	})
}

func (w *Web) cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func (w *Web) setCookie(rw http.ResponseWriter, name, value string, age int) {
	c := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: w.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: age}
	if age < 0 {
		c.Expires = time.Unix(1, 0)
	} else {
		c.Expires = time.Now().Add(time.Duration(age) * time.Second)
	}
	http.SetCookie(rw, c)
}

func (w *Web) Session(r *http.Request) (Session, error) {
	return w.Sessions.Authenticate(r.Context(), w.cookie(r, w.CookieName()))
}

func (w *Web) Protect(mutation bool, next func(http.ResponseWriter, *http.Request, Session)) http.Handler {
	return w.headers(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		session, err := w.Session(r)
		if err != nil {
			http.Error(rw, "Authentication required", http.StatusUnauthorized)
			return
		}
		if mutation {
			if reason := w.originRejection(r); reason != "" {
				rw.Header().Set("X-Request-Rejection", string(reason))
				http.Error(rw, "This request did not come from the application. Reload the page and try again.", http.StatusForbidden)
				return
			}
			if r.ParseForm() != nil {
				rw.Header().Set("X-Request-Rejection", "malformed-form")
				http.Error(rw, "This form could not be read. Reload the page and try again.", http.StatusBadRequest)
				return
			}
			if !security.Equal(r.PostForm.Get("csrf_token"), session.CSRF) {
				rw.Header().Set("X-Request-Rejection", "csrf-invalid")
				http.Error(rw, "This form expired. Reload the page and try again.", http.StatusForbidden)
				return
			}
		}
		next(rw, r, session)
	}))
}

func (w *Web) sameOrigin(r *http.Request) bool {
	return w.originRejection(r) == ""
}

type originRejection string

const (
	originForeign   originRejection = "origin-mismatch"
	originOpaque    originRejection = "origin-opaque"
	originMalformed originRejection = "origin-malformed"
	originMultiple  originRejection = "origin-multiple"
	originFetchSite originRejection = "fetch-site-rejected"
)

func (w *Web) originRejection(r *http.Request) originRejection {
	if origins := r.Header.Values("Origin"); len(origins) > 0 {
		if len(origins) != 1 {
			return originMultiple
		}
		if origins[0] == "null" {
			return originOpaque
		}
		parsed, err := url.Parse(origins[0])
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return originMalformed
		}
		if origins[0] != w.Config.BaseURL {
			return originForeign
		}
		return ""
	}
	if referer := r.Referer(); referer != "" {
		u, err := url.Parse(referer)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
			return originMalformed
		}
		if u.Scheme+"://"+u.Host != w.Config.BaseURL {
			return originForeign
		}
		return ""
	}
	// Referrer-Policy intentionally suppresses Referer, and some browsers or
	// proxies omit Fetch Metadata. In that case the session-bound CSRF token is
	// the mutation proof. Explicit cross-site metadata is still rejected.
	site := r.Header.Get("Sec-Fetch-Site")
	if site == "" || site == "same-origin" {
		return ""
	}
	return originFetchSite
}

func (w *Web) login(rw http.ResponseWriter, r *http.Request) {
	state, verifier, err := w.Sessions.Attempt(r.Context(), r.URL.Query().Get("return_to"))
	if err != nil {
		http.Error(rw, "Authentication unavailable", 503)
		return
	}
	u, err := url.Parse(w.Provider.AuthorizeURL)
	if err != nil {
		http.Error(rw, "Authentication unavailable", 503)
		return
	}
	q := u.Query()
	q.Set("client_id", w.Config.ClientID)
	q.Set("redirect_uri", w.Config.BaseURL+"/auth/github/callback")
	q.Set("scope", "")
	q.Set("state", state)
	q.Set("code_challenge", security.PKCE(verifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	w.setCookie(rw, w.attemptName(), state, int(AttemptLifetime.Seconds()))
	http.Redirect(rw, r, u.String(), http.StatusFound)
}

func (w *Web) callback(rw http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	browser := w.cookie(r, w.attemptName())
	if state == "" || browser == "" || !security.Equal(state, browser) {
		http.Error(rw, "Invalid authentication attempt", http.StatusUnauthorized)
		return
	}
	verifier, path, err := w.Sessions.ConsumeAttempt(r.Context(), state)
	w.setCookie(rw, w.attemptName(), "", -1)
	if err != nil {
		http.Error(rw, "Invalid authentication attempt", http.StatusUnauthorized)
		return
	}
	user, err := w.Provider.Exchange(r.Context(), w.Config, r.URL.Query().Get("code"), verifier)
	if err != nil {
		http.Error(rw, "GitHub authentication failed", http.StatusUnauthorized)
		return
	}
	if w.AfterGitHubLogin != nil {
		if err := w.AfterGitHubLogin(r.Context(), user, security.SafeReturn(path)); err != nil {
			http.Error(rw, "Runner enrollment could not be verified", http.StatusForbidden)
			return
		}
	}
	_, secret, err := w.Sessions.Login(r.Context(), user.GitHubID, user.Username, w.cookie(r, w.CookieName()))
	if err != nil {
		http.Error(rw, "Authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	w.setCookie(rw, w.CookieName(), secret, int(SessionLifetime.Seconds()))
	http.Redirect(rw, r, security.SafeReturn(path), http.StatusSeeOther)
}

func (w *Web) logout(rw http.ResponseWriter, r *http.Request, _ Session) {
	if w.Sessions.Logout(r.Context(), w.cookie(r, w.CookieName())) != nil {
		http.Error(rw, "Logout unavailable", 503)
		return
	}
	w.setCookie(rw, w.CookieName(), "", -1)
	w.setCookie(rw, "bot_space_project", "", -1)
	http.Redirect(rw, r, "/", http.StatusSeeOther)
}

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><title>The Firm</title><body><h1>The Firm</h1>{{if .Session}}<p>Signed in as {{.Session.User.Username}} (GitHub ID {{.Session.User.GitHubID}})</p><form method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.Session.CSRF}}"><button>Log out</button></form><h2>Your workspaces</h2><ul>{{range .Workspaces}}<li><a href="/workspaces/{{.ID}}">{{.Slug}}</a> — {{.Role}}</li>{{else}}<li>No workspace memberships. Ask an administrator for an invitation.</li>{{end}}</ul><h2>Accept an invitation</h2><form method="post" action="/invitations/accept"><input type="hidden" name="csrf_token" value="{{.Session.CSRF}}"><label>Invitation ID <input name="invitation_id" required></label><label>Invitation secret <input name="secret" required autocomplete="off"></label><button>Accept invitation</button></form>{{else}}<p>Private team mailbox.</p><a href="/auth/github/login">Sign in with GitHub</a>{{end}}</body></html>`))

func (w *Web) home(rw http.ResponseWriter, r *http.Request) {
	if w.HomeRenderer != nil {
		session, err := w.Session(r)
		if err != nil {
			w.HomeRenderer(rw, r, nil)
		} else {
			w.HomeRenderer(rw, r, &session)
		}
		return
	}
	var data struct {
		Session    *Session
		Workspaces []workspaces.Workspace
	}
	if session, err := w.Session(r); err == nil {
		list, err := w.Workspaces.List(r.Context(), session.User.ID)
		if err != nil {
			http.Error(rw, "Workspace listing unavailable", 503)
			return
		}
		data.Session = &session
		data.Workspaces = list
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = homeTemplate.Execute(rw, data)
}

var membersTemplate = template.Must(template.New("members").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><title>Workspace members</title><body><a href="/">Workspaces</a><h1>Workspace members</h1><ul>{{range .}}<li>{{.Username}} (GitHub ID {{.GitHubID}}) — {{.Role}}</li>{{end}}</ul></body></html>`))

func (w *Web) workspace(rw http.ResponseWriter, r *http.Request, session Session) {
	if w.WorkspaceRenderer != nil {
		w.WorkspaceRenderer(rw, r, session)
		return
	}
	members, err := w.Workspaces.Members(r.Context(), r.PathValue("workspaceID"), session.User.ID)
	if err != nil {
		http.Error(rw, "Workspace unavailable", http.StatusForbidden)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = membersTemplate.Execute(rw, members)
}

func (w *Web) acceptInvitation(rw http.ResponseWriter, r *http.Request, session Session) {
	workspace, err := w.Workspaces.AcceptInvitation(r.Context(), session.User.ID, r.PostForm.Get("invitation_id"), r.PostForm.Get("secret"))
	if err != nil {
		http.Error(rw, "Invitation cannot be accepted", http.StatusForbidden)
		return
	}
	pendingName := "bot_space_pending_invitation"
	if w.Config.SecureCookies {
		pendingName = "__Host-" + pendingName
	}
	w.setCookie(rw, pendingName, "", -1)
	http.Redirect(rw, r, "/workspaces/"+workspace.ID, http.StatusSeeOther)
}
