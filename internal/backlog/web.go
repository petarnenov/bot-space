package backlog

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
)

type Web struct {
	Store   *Store
	Browser *identity.Web
}
type pageData struct {
	Title, Project, CSRF, Key, Next, Error string
	Current                                Project
	Projects                               []Project
	Items                                  []Intention
	Item                                   *Intention
	History                                []LifecycleEvent
	Progress                               Progress
	Input                                  Input
	Own                                    bool
}

var dashboard = template.Must(template.New("dashboard").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · The Firm</title><link rel="stylesheet" href="/assets/style.css"></head><body><header class="site-header"><a class="brand" href="/">The Firm</a><nav aria-label="Primary navigation"><a aria-current="page" href="/projects/{{.Project}}">Objectives</a><a href="/projects/{{.Project}}/settings">Settings</a><a href="/profile">Profile</a></nav><form class="project-switcher" method="get" action="/"><label for="project-switch">Project</label><select id="project-switch" name="project">{{range .Projects}}<option value="{{.ID}}" {{if eq .ID $.Project}}selected{{end}}>{{.Owner}}/{{.Name}}</option>{{end}}</select><button>Open</button></form><form class="logout" method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button class="secondary">Log out</button></form></header><main><p class="muted">{{.Current.Owner}}/{{.Current.Name}}</p><h1>Objectives</h1><section><h2>New objective</h2>{{if .Error}}<p class="notice" role="alert">{{.Error}}</p>{{end}}<form method="post" action="/projects/{{.Project}}/intentions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="idempotency_key" value="{{.Key}}"><label>Title<input name="title" maxlength="256" value="{{.Input.Title}}" required></label><label>Description<textarea class="objective-description" name="description" rows="18" maxlength="32768" required>{{.Input.Description}}</textarea></label><label>Ticket reference<input name="ticket_reference" maxlength="2048" value="{{.Input.Ticket}}"></label><label>Priority (0–4)<input type="number" name="priority" min="0" max="4" value="{{.Input.Priority}}" required></label><button>Submit objective</button></form></section><section><h2>Project objectives</h2><div class="grid">{{range .Items}}<a class="card" href="/projects/{{.ProjectID}}/intentions/{{.ID}}"><h3>{{.Title}}</h3><span class="badge">{{.State}}</span><p class="muted">Revision {{.Revision}}</p></a>{{else}}<div class="empty"><h3>No objectives yet</h3><p>Use the form above to give the system its first objective.</p></div>{{end}}</div>{{if .Next}}<a href="/projects/{{.Project}}?after={{.Next}}">Next page</a>{{end}}</section></main></body></html>`))

var detail = template.Must(template.New("detail").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Item.Title}} · The Firm</title><link rel="stylesheet" href="/assets/style.css"></head><body><header class="site-header"><a class="brand" href="/">The Firm</a><nav aria-label="Primary navigation"><a aria-current="page" href="/projects/{{.Project}}">Objectives</a><a href="/projects/{{.Project}}/settings">Settings</a><a href="/profile">Profile</a></nav><form class="logout" method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button class="secondary">Log out</button></form></header><main><a href="/projects/{{.Project}}">← Objectives</a><h1>{{.Item.Title}}</h1><section><span class="badge">{{.Item.State}}</span><p>{{.Item.Description}}</p>{{if .Item.Ticket}}<p>Ticket: {{.Item.Ticket}}</p>{{end}}<dl><dt>Progress</dt><dd>{{.Progress.Phase}}</dd><dt>Result</dt><dd>{{.Progress.Outcome}}</dd><dt>Revision</dt><dd>{{.Item.Revision}}</dd></dl>{{if .Item.Archived}}<p>Archived — history is preserved.</p>{{end}}{{if .Item.ReconciliationRequired}}<p class="notice">Execution is fenced pending architect reconciliation.</p>{{end}}</section>{{if .Own}}{{if not .Item.Archived}}<section><h2>Controls</h2><form method="post" action="/projects/{{.Project}}/intentions/{{.Item.ID}}/control"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="expected_epoch" value="{{.Item.Epoch}}">{{if eq .Item.State "paused"}}<button name="action" value="resume">Resume for architect reconciliation</button>{{else if and (ne .Item.State "cancelled") (ne .Item.State "completed")}}<button name="action" value="pause">Pause</button>{{end}}{{if and (ne .Item.State "cancelled") (ne .Item.State "completed")}}<button class="danger" name="action" value="cancel">Cancel objective</button>{{end}}{{if or (eq .Item.State "paused") (eq .Item.State "cancelled") (eq .Item.State "completed")}}<button name="action" value="archive">Archive and preserve history</button>{{end}}</form></section>{{end}}{{end}}{{if and .Own (not .Item.Archived) (ne .Item.State "cancelled") (ne .Item.State "completed")}}<details><summary>Edit objective input</summary><form method="post" action="/projects/{{.Project}}/intentions/{{.Item.ID}}/revisions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="idempotency_key" value="revision-input"><input type="hidden" name="expected_revision" value="{{.Item.Revision}}"><label>Title<input name="title" value="{{.Item.Title}}" required></label><label>Description<textarea class="objective-description" rows="18" name="description" required>{{.Item.Description}}</textarea></label><label>Ticket reference<input name="ticket_reference" value="{{.Item.Ticket}}"></label><label>Priority<input type="number" name="priority" min="0" max="4" value="{{.Item.Priority}}"></label><button>Save new input revision</button></form></details>{{end}}<section><h2>History</h2><ol>{{range .History}}<li>{{.CreatedAt}} — {{.Action}}: {{.Previous}} → {{.State}} (epoch {{.Epoch}})</li>{{else}}<li>No lifecycle changes.</li>{{end}}</ol></section><details><summary>Technical evidence</summary><dl><dt>Council decisions</dt><dd>{{.Progress.Decisions}}</dd><dt>Assignments</dt><dd>{{.Progress.Assignments}}</dd><dt>Completed assignments</dt><dd>{{.Progress.Done}}</dd><dt>Execution attempts</dt><dd>{{.Progress.Attempts}}</dd></dl></details></main></body></html>`))

func (w *Web) secret(r *http.Request) string {
	c, e := r.Cookie(w.Browser.CookieName())
	if e != nil {
		return ""
	}
	return c.Value
}
func (w *Web) Register(routes identity.Routes) {
	routes.Handle("GET /projects/{project}", w.Browser.Protect(false, w.list))
	routes.Handle("GET /projects/{project}/intentions", w.Browser.Protect(false, w.legacyList))
	routes.Handle("POST /projects/{project}/intentions", w.Browser.Protect(true, w.create))
	routes.Handle("GET /projects/{project}/intentions/{id}", w.Browser.Protect(false, w.show))
	routes.Handle("POST /projects/{project}/intentions/{id}/control", w.Browser.Protect(true, w.control))
	routes.Handle("POST /projects/{project}/intentions/{id}/revisions", w.Browser.Protect(true, w.revise))
}
func setCookie(rw http.ResponseWriter, project string, secure bool) {
	http.SetCookie(rw, &http.Cookie{Name: "bot_space_project", Value: project, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
}
func (w *Web) page(r *http.Request, s identity.Session, in Input, message string) (pageData, error) {
	p, err := w.Store.Project(r.Context(), w.secret(r), r.PathValue("project"))
	if err != nil {
		return pageData{}, err
	}
	items, err := w.Store.List(r.Context(), w.secret(r), p.ID, r.URL.Query().Get("after"))
	if err != nil {
		return pageData{}, err
	}
	projects, err := w.Store.Projects(r.Context(), w.secret(r))
	if err != nil {
		return pageData{}, err
	}
	key := in.Key
	if key == "" {
		key, err = security.Secret()
		if err != nil {
			return pageData{}, ErrUnavailable
		}
	}
	if in.Priority < 0 || in.Priority > 4 {
		in.Priority = 0
	}
	next := ""
	if len(items) == 50 {
		next = items[len(items)-1].ID
	}
	return pageData{Title: "Objectives", Project: p.ID, CSRF: s.CSRF, Key: key, Current: p, Projects: projects, Items: items, Next: next, Input: in, Error: message}, nil
}
func (w *Web) list(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	data, err := w.page(r, s, Input{}, "")
	if err != nil {
		webError(rw, err, "Project objectives unavailable")
		return
	}
	setCookie(rw, data.Project, w.Browser.Config.SecureCookies)
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dashboard.Execute(rw, data)
}
func (w *Web) legacyList(rw http.ResponseWriter, r *http.Request, _ identity.Session) {
	if _, err := w.Store.Project(r.Context(), w.secret(r), r.PathValue("project")); err != nil {
		webError(rw, err, "Project unavailable")
		return
	}
	after := r.URL.Query().Get("after")
	if after != "" && !security.ValidUUID(after) {
		http.Error(rw, "Invalid page cursor", 400)
		return
	}
	target := "/projects/" + r.PathValue("project")
	if after != "" {
		target += "?" + url.Values{"after": {after}}.Encode()
	}
	http.Redirect(rw, r, target, http.StatusTemporaryRedirect)
}
func input(r *http.Request) (Input, error) {
	p, e := strconv.Atoi(r.PostForm.Get("priority"))
	if e != nil {
		return Input{}, ErrInvalid
	}
	return Input{ProjectID: r.PathValue("project"), Key: r.PostForm.Get("idempotency_key"), Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"), Ticket: r.PostForm.Get("ticket_reference"), Priority: p}, nil
}
func (w *Web) create(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	in, err := input(r)
	if err == nil {
		_, err = normalize(in)
	}
	if err != nil {
		data, e := w.page(r, s, in, "Check the required fields and try again.")
		if e != nil {
			webError(rw, e, "Objective could not be submitted")
			return
		}
		rw.WriteHeader(400)
		_ = dashboard.Execute(rw, data)
		return
	}
	item, err := w.Store.Create(r.Context(), w.secret(r), in)
	if err != nil {
		if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
			data, e := w.page(r, s, in, "This objective conflicts with an earlier submission. Review it and try again.")
			if e == nil {
				rw.WriteHeader(status(err))
				_ = dashboard.Execute(rw, data)
				return
			}
		}
		webError(rw, err, "Objective could not be submitted")
		return
	}
	http.Redirect(rw, r, "/projects/"+item.ProjectID+"/intentions/"+item.ID, 303)
}
func (w *Web) show(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	item, err := w.Store.Get(r.Context(), w.secret(r), r.PathValue("project"), r.PathValue("id"), 0)
	if err != nil {
		webError(rw, err, "Objective unavailable")
		return
	}
	history, err := w.Store.History(r.Context(), w.secret(r), item.ProjectID, item.ID)
	if err != nil {
		webError(rw, err, "History unavailable")
		return
	}
	progress, err := w.Store.Progress(r.Context(), w.secret(r), item.ProjectID, item.ID)
	if err != nil {
		webError(rw, err, "Progress unavailable")
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = detail.Execute(rw, pageData{Title: item.Title, Project: item.ProjectID, CSRF: s.CSRF, Item: &item, Own: item.Creator == s.User.ID, History: history, Progress: progress})
}
func (w *Web) revise(rw http.ResponseWriter, r *http.Request, _ identity.Session) {
	in, err := input(r)
	expected, e := strconv.Atoi(r.PostForm.Get("expected_revision"))
	if err != nil || e != nil {
		http.Error(rw, "Invalid revision", 400)
		return
	}
	item, err := w.Store.Revise(r.Context(), w.secret(r), r.PathValue("id"), expected, in)
	if err != nil {
		webError(rw, err, "Revision could not be saved")
		return
	}
	http.Redirect(rw, r, "/projects/"+item.ProjectID+"/intentions/"+item.ID, 303)
}
func (w *Web) control(rw http.ResponseWriter, r *http.Request, _ identity.Session) {
	epoch, err := strconv.ParseInt(r.PostForm.Get("expected_epoch"), 10, 64)
	if err != nil {
		http.Error(rw, "Invalid lifecycle", 400)
		return
	}
	item, err := w.Store.Control(r.Context(), w.secret(r), r.PathValue("project"), r.PathValue("id"), r.PostForm.Get("action"), epoch)
	if err != nil {
		webError(rw, err, "Objective control rejected")
		return
	}
	http.Redirect(rw, r, "/projects/"+item.ProjectID+"/intentions/"+item.ID, 303)
}
func status(err error) int {
	switch {
	case errors.Is(err, ErrInvalid):
		return 400
	case errors.Is(err, ErrForbidden):
		return 403
	case errors.Is(err, ErrConflict):
		return 409
	default:
		return 503
	}
}
func webError(rw http.ResponseWriter, err error, message string) {
	if errors.Is(err, ErrForbidden) {
		message = "Project access is unavailable. Return home and choose an authorized project."
	}
	if errors.Is(err, ErrInvalid) {
		message = "Invalid request"
	}
	http.Error(rw, strings.TrimSpace(message), status(err))
}
