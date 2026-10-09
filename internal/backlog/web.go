package backlog

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
)

type Web struct {
	Store   *Store
	Browser *identity.Web
}

var intake = template.Must(template.New("backlog").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Project backlog · The Firm</title><link rel="stylesheet" href="/assets/style.css"></head><body><header><a href="/">The Firm</a></header><main><h1>Project backlog</h1><form method="post" action="/projects/{{.Project}}/intentions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="idempotency_key" value="{{.Key}}"><label>Title <input name="title" maxlength="256" required></label><label>Description <textarea name="description" required></textarea></label><label>Ticket reference <input name="ticket_reference" maxlength="2048"></label><label>Priority (0–4) <input type="number" name="priority" min="0" max="4" value="0" required></label><button>Submit objective</button></form><ul>{{range .Items}}<li><a href="/projects/{{.ProjectID}}/intentions/{{.ID}}">{{.Title}}</a> — {{.State}} · revision {{.Revision}}</li>{{else}}<li>No objectives yet.</li>{{end}}</ul>{{if .Next}}<a href="/projects/{{.Project}}/intentions?after={{.Next}}">Next page</a>{{end}}</main></body></html>`))
var detail = template.Must(template.New("intention").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>{{.Item.Title}} · The Firm</title></head><body><main><h1>{{.Item.Title}}</h1><p>{{.Item.Description}}</p><p>Ticket: {{.Item.Ticket}}</p><p>State: {{.Item.State}} · revision {{.Item.Revision}} · lifecycle {{.Item.Epoch}}</p>{{if .Item.Archived}}<p>Archived — history is preserved.</p>{{end}}{{if .Item.ReconciliationRequired}}<p>Execution is fenced pending architect reconciliation.</p>{{end}}{{if .Own}}{{if not .Item.Archived}}<form method="post" action="/projects/{{.Item.ProjectID}}/intentions/{{.Item.ID}}/control"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="expected_epoch" value="{{.Item.Epoch}}">{{if eq .Item.State "paused"}}<button name="action" value="resume">Resume for architect reconciliation</button>{{else}}{{if and (ne .Item.State "cancelled") (ne .Item.State "completed")}}<button name="action" value="pause">Pause</button>{{end}}{{end}}{{if and (ne .Item.State "cancelled") (ne .Item.State "completed")}}<button name="action" value="cancel">Cancel objective</button>{{end}}{{if or (eq .Item.State "paused") (eq .Item.State "cancelled") (eq .Item.State "completed")}}<button name="action" value="archive">Archive and preserve history</button>{{end}}</form>{{end}}{{end}}{{if and .Own (not .Item.Archived) (ne .Item.State "cancelled") (ne .Item.State "completed")}}<form method="post" action="/projects/{{.Item.ProjectID}}/intentions/{{.Item.ID}}/revisions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="idempotency_key" value="revision-input"><input type="hidden" name="expected_revision" value="{{.Item.Revision}}"><label>Title <input name="title" value="{{.Item.Title}}" required></label><label>Description <textarea name="description" required>{{.Item.Description}}</textarea></label><label>Ticket reference <input name="ticket_reference" value="{{.Item.Ticket}}"></label><label>Priority <input type="number" name="priority" min="0" max="4" value="{{.Item.Priority}}"></label><button>Save new input revision</button></form>{{end}}<h2>Lifecycle history</h2><ol>{{range .History}}<li>{{.CreatedAt}} — {{.Action}} by {{.Actor}}: {{.Previous}} → {{.State}} (epoch {{.Epoch}})</li>{{else}}<li>No lifecycle changes.</li>{{end}}</ol><a href="/projects/{{.Item.ProjectID}}/intentions">Back to backlog</a></main></body></html>`))

func (w *Web) secret(r *http.Request) string {
	c, err := r.Cookie(w.Browser.CookieName())
	if err != nil {
		return ""
	}
	return c.Value
}
func (w *Web) Register(routes identity.Routes) {
	routes.Handle("GET /projects/{project}/intentions", w.Browser.Protect(false, w.list))
	routes.Handle("POST /projects/{project}/intentions", w.Browser.Protect(true, w.create))
	routes.Handle("GET /projects/{project}/intentions/{id}", w.Browser.Protect(false, w.show))
	routes.Handle("POST /projects/{project}/intentions/{id}/control", w.Browser.Protect(true, w.control))
	routes.Handle("POST /projects/{project}/intentions/{id}/revisions", w.Browser.Protect(true, w.revise))
}
func (w *Web) list(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	items, err := w.Store.List(r.Context(), w.secret(r), r.PathValue("project"), r.URL.Query().Get("after"))
	if err != nil {
		http.Error(rw, "Project backlog unavailable", 403)
		return
	}
	key, err := security.Secret()
	if err != nil {
		http.Error(rw, "Intake unavailable", 503)
		return
	}
	next := ""
	if len(items) == 50 {
		next = items[len(items)-1].ID
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = intake.Execute(rw, struct {
		Project, CSRF, Key, Next string
		Items                    []Intention
	}{r.PathValue("project"), s.CSRF, key, next, items})
}
func input(r *http.Request) (Input, error) {
	priority, err := strconv.Atoi(r.PostForm.Get("priority"))
	if err != nil {
		return Input{}, ErrInvalid
	}
	return Input{ProjectID: r.PathValue("project"), Key: r.PostForm.Get("idempotency_key"), Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"), Ticket: r.PostForm.Get("ticket_reference"), Priority: priority}, nil
}
func (w *Web) create(rw http.ResponseWriter, r *http.Request, _ identity.Session) {
	in, err := input(r)
	if err != nil {
		http.Error(rw, "Invalid objective", 400)
		return
	}
	item, err := w.Store.Create(r.Context(), w.secret(r), in)
	if err != nil {
		http.Error(rw, "Objective could not be submitted", 400)
		return
	}
	http.Redirect(rw, r, "/projects/"+item.ProjectID+"/intentions/"+item.ID, 303)
}
func (w *Web) show(rw http.ResponseWriter, r *http.Request, s identity.Session) {
	item, err := w.Store.Get(r.Context(), w.secret(r), r.PathValue("project"), r.PathValue("id"), 0)
	if err != nil {
		http.Error(rw, "Objective unavailable", 403)
		return
	}
	history, err := w.Store.History(r.Context(), w.secret(r), item.ProjectID, item.ID)
	if err != nil {
		http.Error(rw, "History unavailable", 503)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = detail.Execute(rw, struct {
		Item    Intention
		Own     bool
		CSRF    string
		History []LifecycleEvent
	}{item, item.Creator == s.User.ID, s.CSRF, history})
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
		http.Error(rw, "Revision could not be saved", 409)
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
		http.Error(rw, "Objective control rejected", 409)
		return
	}
	http.Redirect(rw, r, "/projects/"+item.ProjectID+"/intentions/"+item.ID, 303)
}
