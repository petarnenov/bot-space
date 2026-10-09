// Package web renders private administration with the existing server policies.
package web

import (
	"context"
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
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func setProjectCookie(rw http.ResponseWriter, project string, secure bool) {
	http.SetCookie(rw, &http.Cookie{Name: "bot_space_project", Value: project, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
}

//go:embed assets/* templates/*
var content embed.FS

type Management struct {
	Browser        *identity.Web
	Teams          *workspaces.Store
	Agents         *agents.Store
	Projects       *backlog.Store
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
	Projects                                       []backlog.Project
	CurrentProject                                 *backlog.Project
	Active                                         string
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
	routes.Handle("GET /profile", m.Browser.Protect(false, m.profile))
	routes.Handle("GET /settings", m.Browser.Protect(false, m.settings))
	routes.Handle("GET /projects/{project}/settings", m.Browser.Protect(false, m.projectSettings))
	routes.Handle("GET /workspaces/{workspaceID}/settings", m.Browser.Protect(false, m.workspace))
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
	data := page{Title: "The Firm", Session: session, MailboxEnabled: m.MailboxEnabled, Active: "objectives"}
	if session == nil {
		render(rw, data)
		return
	}
	if m.Projects == nil {
		list, err := m.Teams.List(r.Context(), session.User.ID)
		if err != nil {
			http.Error(rw, "Workspace listing unavailable", http.StatusServiceUnavailable)
			return
		}
		data.Workspaces = list
		data.Title = "Workspaces"
		render(rw, data)
		return
	}
	projects, err := m.Projects.Projects(r.Context(), m.secret(r))
	if err != nil {
		http.Error(rw, "Project listing unavailable", http.StatusServiceUnavailable)
		return
	}
	data.Projects = projects
	if requested := strings.ToLower(r.URL.Query().Get("project")); requested != "" {
		if !security.ValidUUID(requested) {
			http.Error(rw, "Invalid project selection", http.StatusBadRequest)
			return
		}
		for i := range projects {
			if projects[i].ID == requested {
				setProjectCookie(rw, requested, m.Browser.Config.SecureCookies)
				http.Redirect(rw, r, "/projects/"+requested, http.StatusSeeOther)
				return
			}
		}
		http.Error(rw, "Project unavailable", http.StatusForbidden)
		return
	}
	selected := selectedProject(r)
	for i := range projects {
		if projects[i].ID == selected {
			http.Redirect(rw, r, "/projects/"+projects[i].ID, http.StatusSeeOther)
			return
		}
	}
	if len(projects) == 1 {
		http.Redirect(rw, r, "/projects/"+projects[0].ID, http.StatusSeeOther)
		return
	}
	render(rw, data)
}

func (m *Management) secret(r *http.Request) string {
	cookie, err := r.Cookie(m.Browser.CookieName())
	if err != nil {
		return ""
	}
	return cookie.Value
}

func selectedProject(r *http.Request) string {
	cookie, err := r.Cookie("bot_space_project")
	if err != nil || !security.ValidUUID(cookie.Value) {
		return ""
	}
	return strings.ToLower(cookie.Value)
}

func (m *Management) navigation(ctx context.Context, r *http.Request, data *page) error {
	if data.Session == nil || m.Projects == nil {
		return nil
	}
	projects, err := m.Projects.Projects(ctx, m.secret(r))
	if err != nil {
		return err
	}
	data.Projects = projects
	selected := selectedProject(r)
	for i := range projects {
		if projects[i].ID == selected {
			data.CurrentProject = &projects[i]
			break
		}
	}
	return nil
}

func (m *Management) profile(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	data := page{Title: "Profile", Session: &session, Active: "profile", MailboxEnabled: m.MailboxEnabled}
	if err := m.navigation(r.Context(), r, &data); err != nil {
		http.Error(rw, "Profile navigation unavailable", http.StatusServiceUnavailable)
		return
	}
	render(rw, data)
}

func (m *Management) settings(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	list, err := m.Teams.List(r.Context(), session.User.ID)
	if err != nil {
		http.Error(rw, "Workspace listing unavailable", http.StatusServiceUnavailable)
		return
	}
	data := page{Title: "Settings", Session: &session, Workspaces: list, Active: "settings", MailboxEnabled: m.MailboxEnabled}
	if err := m.navigation(r.Context(), r, &data); err != nil {
		http.Error(rw, "Settings unavailable", http.StatusServiceUnavailable)
		return
	}
	render(rw, data)
}

func (m *Management) projectSettings(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	if m.Projects == nil {
		http.NotFound(rw, r)
		return
	}
	project, err := m.Projects.Project(r.Context(), m.secret(r), r.PathValue("project"))
	if err != nil {
		http.Error(rw, "Project settings unavailable", http.StatusForbidden)
		return
	}
	setProjectCookie(rw, project.ID, m.Browser.Config.SecureCookies)
	http.Redirect(rw, r, "/workspaces/"+project.WorkspaceID+"/settings", http.StatusSeeOther)
}

func (m *Management) workspace(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	if r.URL.Path == "/workspaces/"+r.PathValue("workspaceID") {
		if _, err := m.Teams.Get(r.Context(), r.PathValue("workspaceID"), s.User.ID); err != nil {
			http.Error(rw, "Workspace unavailable", http.StatusForbidden)
			return
		}
		http.Redirect(rw, r, "/workspaces/"+r.PathValue("workspaceID")+"/settings", http.StatusTemporaryRedirect)
		return
	}
	w, err := m.Teams.Get(r.Context(), r.PathValue("workspaceID"), s.User.ID)
	if err != nil {
		http.Error(rw, "Workspace unavailable", 403)
		return
	}
	data := page{Title: w.Slug + " settings", Session: &s, Workspace: &w, Admin: w.Role == "owner" || w.Role == "admin", Owner: w.Role == "owner", MailboxEnabled: m.MailboxEnabled, Active: "settings"}
	if err := m.navigation(r.Context(), r, &data); err != nil {
		http.Error(rw, "Settings navigation unavailable", http.StatusServiceUnavailable)
		return
	}
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
	http.Redirect(rw, r, "/workspaces/"+r.PathValue("workspaceID")+"/settings", 303)
}
func (m *Management) removeMember(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	if err := m.Teams.Remove(r.Context(), r.PathValue("workspaceID"), s.User.ID, r.PathValue("userID")); err != nil {
		actionError(rw, err)
		return
	}
	http.Redirect(rw, r, "/settings", 303)
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
	http.Redirect(rw, r, "/workspaces/"+r.PathValue("workspaceID")+"/settings", 303)
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
