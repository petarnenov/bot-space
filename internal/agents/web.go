package agents

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/petarnenov/bot-space/internal/identity"
)

type Web struct{ Store *Store }
type resultPage struct{ Title, WorkspaceID, AgentID, CredentialID, Token string }

var resultTemplate = template.Must(template.New("agent-result").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title><link rel="stylesheet" href="/assets/style.css"></head><body><header><a class="brand" href="/">bot-space</a></header><main><section><h1>{{.Title}}</h1><dl><dt>Agent ID</dt><dd><code>{{.AgentID}}</code></dd>{{if .CredentialID}}<dt>Credential ID</dt><dd><code>{{.CredentialID}}</code></dd>{{end}}</dl>{{if .Token}}<p>Save this token now. It will not be shown again.</p><code id="issued-token">{{.Token}}</code>{{end}}<p><a href="/workspaces/{{.WorkspaceID}}">Return to workspace</a></p></section></main></body></html>`))

func (w *Web) Register(routes identity.Routes, browser *identity.Web) {
	routes.Handle("POST /workspaces/{workspaceID}/agents", browser.Protect(true, w.register))
	routes.Handle("POST /workspaces/{workspaceID}/agents/{agentID}/deactivate", browser.Protect(true, w.deactivate))
	routes.Handle("POST /workspaces/{workspaceID}/agents/{agentID}/credentials", browser.Protect(true, w.issue))
	routes.Handle("POST /workspaces/{workspaceID}/agents/{agentID}/credentials/{credentialID}/revoke", browser.Protect(true, w.revoke))
	routes.Handle("POST /workspaces/{workspaceID}/agents/{agentID}/credentials/{credentialID}/rotate", browser.Protect(true, w.rotate))
}

func render(rw http.ResponseWriter, data resultPage) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = resultTemplate.Execute(rw, data)
}

func failure(rw http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	message := "Operation unavailable"
	switch {
	case errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
		message = "Invalid agent configuration"
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
		message = "Forbidden"
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		message = "Agent or credential unavailable"
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
		message = "Agent or credential is inactive"
	}
	http.Error(rw, message, status)
}

func (w *Web) register(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	a, err := w.Store.Register(r.Context(), r.PathValue("workspaceID"), session.User.ID, r.PostForm.Get("name"))
	if err != nil {
		failure(rw, err)
		return
	}
	render(rw, resultPage{Title: "Agent registered", WorkspaceID: a.WorkspaceID, AgentID: a.ID})
}

func (w *Web) deactivate(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	workspaceID, agentID := r.PathValue("workspaceID"), r.PathValue("agentID")
	if err := w.Store.Deactivate(r.Context(), workspaceID, session.User.ID, agentID); err != nil {
		failure(rw, err)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+workspaceID, http.StatusSeeOther)
}

func (w *Web) issue(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	workspaceID, agentID := r.PathValue("workspaceID"), r.PathValue("agentID")
	c, token, err := w.Store.Issue(r.Context(), workspaceID, session.User.ID, agentID)
	if err != nil {
		failure(rw, err)
		return
	}
	render(rw, resultPage{Title: "Credential issued", WorkspaceID: workspaceID, AgentID: agentID, CredentialID: c.ID, Token: token})
}

func (w *Web) revoke(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	workspaceID, agentID, credentialID := r.PathValue("workspaceID"), r.PathValue("agentID"), r.PathValue("credentialID")
	if err := w.Store.Revoke(r.Context(), workspaceID, session.User.ID, agentID, credentialID); err != nil {
		failure(rw, err)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+workspaceID, http.StatusSeeOther)
}

func (w *Web) rotate(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	workspaceID, agentID, credentialID := r.PathValue("workspaceID"), r.PathValue("agentID"), r.PathValue("credentialID")
	c, token, err := w.Store.Rotate(r.Context(), workspaceID, session.User.ID, agentID, credentialID)
	if err != nil {
		failure(rw, err)
		return
	}
	render(rw, resultPage{Title: "Credential rotated", WorkspaceID: workspaceID, AgentID: agentID, CredentialID: c.ID, Token: token})
}
