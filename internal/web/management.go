// Package web renders private administration with the existing server policies.
package web

import (
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

//go:embed assets/* templates/*
var content embed.FS

type Management struct {
	Browser        *identity.Web
	Teams          *workspaces.Store
	Agents         *agents.Store
	MailboxEnabled bool
}
type agentView struct {
	agents.Agent
	Credentials []agents.Credential
}
type page struct {
	Title                                          string
	Session                                        *identity.Session
	Workspace                                      *workspaces.Workspace
	Workspaces                                     []workspaces.Workspace
	Members                                        []workspaces.Member
	Invitations                                    []workspaces.Invitation
	Agents                                         []agentView
	TeamAgents                                     []workspaces.TeamAgent
	Admin, Owner, MailboxEnabled                   bool
	Invitation                                     *workspaces.Invitation
	InvitationID, InvitationSecret, InvitationLink string
}

var pages = template.Must(template.New("pages.html").ParseFS(content, "templates/pages.html"))

func (m *Management) Register(routes identity.Routes) {
	m.Browser.HomeRenderer = m.home
	m.Browser.WorkspaceRenderer = m.workspace
	static, _ := fs.Sub(content, "assets")
	routes.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	routes.Handle("POST /workspaces/{workspaceID}/members/{userID}/role", m.Browser.Protect(true, m.setRole))
	routes.Handle("POST /workspaces/{workspaceID}/members/{userID}/remove", m.Browser.Protect(true, m.removeMember))
	routes.Handle("POST /workspaces/{workspaceID}/invitations", m.Browser.Protect(true, m.invite))
	routes.Handle("POST /workspaces/{workspaceID}/invitations/{invitationID}/cancel", m.Browser.Protect(true, m.cancelInvite))
	routes.Handle("GET /invitations/accept", http.HandlerFunc(m.invitationPage))
}

func render(rw http.ResponseWriter, data page) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	_ = pages.Execute(rw, data)
}

func (m *Management) home(rw http.ResponseWriter, r *http.Request, session *identity.Session) {
	data := page{Title: "Your workspaces", Session: session, MailboxEnabled: m.MailboxEnabled}
	if session != nil {
		list, err := m.Teams.List(r.Context(), session.User.ID)
		if err != nil {
			http.Error(rw, "Workspace listing unavailable", 503)
			return
		}
		data.Workspaces = list
	}
	render(rw, data)
}

func (m *Management) workspace(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	w, err := m.Teams.Get(r.Context(), r.PathValue("workspaceID"), s.User.ID)
	if err != nil {
		http.Error(rw, "Workspace unavailable", 403)
		return
	}
	data := page{Title: w.Slug, Session: &s, Workspace: &w, Admin: w.Role == "owner" || w.Role == "admin", Owner: w.Role == "owner", MailboxEnabled: m.MailboxEnabled}
	data.Members, err = m.Teams.Members(r.Context(), w.ID, s.User.ID)
	if err != nil {
		http.Error(rw, "Members unavailable", 503)
		return
	}
	own, err := m.Agents.Own(r.Context(), w.ID, s.User.ID)
	if err != nil {
		http.Error(rw, "Agents unavailable", 503)
		return
	}
	for _, a := range own {
		credentials, err := m.Agents.Credentials(r.Context(), w.ID, s.User.ID, a.ID)
		if err != nil {
			http.Error(rw, "Credential metadata unavailable", 503)
			return
		}
		data.Agents = append(data.Agents, agentView{a, credentials})
	}
	if data.Admin {
		data.Invitations, err = m.Teams.Invitations(r.Context(), w.ID, s.User.ID)
		if err != nil {
			http.Error(rw, "Invitations unavailable", 503)
			return
		}
		data.TeamAgents, err = m.Teams.TeamAgents(r.Context(), w.ID, s.User.ID)
		if err != nil {
			http.Error(rw, "Team agents unavailable", 503)
			return
		}
	}
	render(rw, data)
}

func actionError(rw http.ResponseWriter, err error) {
	status, message := 503, "Operation unavailable"
	switch {
	case errors.Is(err, workspaces.ErrForbidden):
		status = 403
		message = "Forbidden"
	case errors.Is(err, workspaces.ErrConflict):
		status = 409
		message = "Operation conflicts with current membership or ownership"
	case errors.Is(err, workspaces.ErrInvalid):
		status = 400
		message = "Invalid request"
	case errors.Is(err, workspaces.ErrNotFound):
		status = 404
		message = "Record unavailable"
	}
	http.Error(rw, message, status)
}
func (m *Management) setRole(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	if err := m.Teams.SetRole(r.Context(), r.PathValue("workspaceID"), s.User.ID, r.PathValue("userID"), r.PostForm.Get("role")); err != nil {
		actionError(rw, err)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+r.PathValue("workspaceID"), 303)
}
func (m *Management) removeMember(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	if err := m.Teams.Remove(r.Context(), r.PathValue("workspaceID"), s.User.ID, r.PathValue("userID")); err != nil {
		actionError(rw, err)
		return
	}
	http.Redirect(rw, r, "/", 303)
}
func (m *Management) invite(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	target, err := strconv.ParseInt(r.PostForm.Get("github_user_id"), 10, 64)
	if err != nil {
		actionError(rw, workspaces.ErrInvalid)
		return
	}
	i, secret, err := m.Teams.Invite(r.Context(), r.PathValue("workspaceID"), s.User.ID, target, r.PostForm.Get("role"))
	if err != nil {
		actionError(rw, err)
		return
	}
	link := m.Browser.Config.BaseURL + "/invitations/accept?" + url.Values{"invitation_id": {i.ID}, "secret": {secret}}.Encode()
	render(rw, page{Title: "Invitation created", Session: &s, Invitation: &i, InvitationID: i.ID, InvitationSecret: secret, InvitationLink: link})
}
func (m *Management) cancelInvite(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	if err := m.Teams.CancelInvitation(r.Context(), r.PathValue("workspaceID"), s.User.ID, r.PathValue("invitationID")); err != nil {
		actionError(rw, err)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+r.PathValue("workspaceID"), 303)
}

func (m *Management) invitationPage(rw http.ResponseWriter, r *http.Request) {
	id, secret := r.URL.Query().Get("invitation_id"), r.URL.Query().Get("secret")
	name := "bot_space_pending_invitation"
	if m.Browser.Config.SecureCookies {
		name = "__Host-" + name
	}
	if security.ValidUUID(id) && len(secret) == 43 {
		http.SetCookie(rw, &http.Cookie{Name: name, Value: id + "." + secret, Path: "/", MaxAge: 600, Expires: time.Now().Add(10 * time.Minute), HttpOnly: true, Secure: m.Browser.Config.SecureCookies, SameSite: http.SameSiteLaxMode})
	} else if cookie, err := r.Cookie(name); err == nil {
		parts := strings.Split(cookie.Value, ".")
		if len(parts) == 2 && security.ValidUUID(parts[0]) && len(parts[1]) == 43 {
			id, secret = parts[0], parts[1]
		}
	}
	var session *identity.Session
	if s, err := m.Browser.Session(r); err == nil {
		session = &s
	}
	render(rw, page{Title: "Accept invitation", Session: session, InvitationID: id, InvitationSecret: secret})
}
