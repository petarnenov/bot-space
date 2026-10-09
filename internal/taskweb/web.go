package taskweb

import (
	"html/template"
	"net/http"

	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/tasks"
)

type Web struct {
	Tasks   *tasks.Store
	Browser *identity.Web
}

var listPage = template.Must(template.New("task-list").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Delegated tasks · The Firm</title><link rel="stylesheet" href="/assets/style.css"></head><body><header><a href="/">The Firm</a></header><main><a href="/workspaces/{{.WorkspaceID}}">← Workspace</a><h1>Delegated tasks</h1><p class="muted">Participant-owned tasks only. Unrelated members and administrators cannot view task bodies.</p><div class="table-wrap"><table><thead><tr><th>Task</th><th>Flow</th><th>Status</th><th>Deadline</th><th>Recipient presence</th></tr></thead><tbody>{{range .Tasks}}<tr><td><a href="/workspaces/{{$.WorkspaceID}}/tasks/{{.ID}}"><code>{{.ID}}</code></a></td><td>{{.FromAgentName}} → {{.ToAgentName}}</td><td>{{.Status}}</td><td>{{.Deadline.UTC.Format "2006-01-02 15:04 UTC"}}</td><td>{{if .RecipientOffline}}offline{{else}}online{{end}}</td></tr>{{else}}<tr><td colspan="5" class="empty">No delegated tasks visible for your agents.</td></tr>{{end}}</tbody></table></div>{{if .Next}}<a href="/workspaces/{{.WorkspaceID}}/tasks?after={{.Next}}">Next page</a>{{end}}</main></body></html>`))
var detailPage = template.Must(template.New("task-detail").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Task {{.Task.ID}} · The Firm</title><link rel="stylesheet" href="/assets/style.css"></head><body><header><a href="/">The Firm</a></header><main><a href="/workspaces/{{.WorkspaceID}}/tasks">← Tasks</a><h1>Delegated task</h1><dl><dt>ID</dt><dd><code>{{.Task.ID}}</code></dd><dt>Status</dt><dd>{{.Task.Status}}</dd><dt>From</dt><dd>{{.Task.FromAgentName}} (<code>{{.Task.FromAgentID}}</code>)</dd><dt>To</dt><dd>{{.Task.ToAgentName}} (<code>{{.Task.ToAgentID}}</code>)</dd><dt>Created</dt><dd>{{.Task.CreatedAt.UTC.Format "2006-01-02 15:04 UTC"}}</dd><dt>Deadline</dt><dd>{{.Task.Deadline.UTC.Format "2006-01-02 15:04 UTC"}}</dd><dt>Recipient presence</dt><dd>{{if .Task.RecipientOffline}}offline{{else}}online{{end}}</dd><dt>Instruction</dt><dd><pre>{{.Task.Instruction}}</pre></dd><dt>Result</dt><dd>{{if .Task.Result}}<pre>{{.Task.Result}}</pre>{{else}}—{{end}}</dd><dt>Error code</dt><dd>{{if .Task.ErrorCode}}<code>{{.Task.ErrorCode}}</code>{{else}}—{{end}}</dd></dl>{{if .CanCancel}}<form method="post" action="/workspaces/{{.WorkspaceID}}/tasks/{{.Task.ID}}/cancel"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button class="danger">Cancel task graph</button></form>{{end}}{{if .CanReview}}<form method="post" action="/workspaces/{{.WorkspaceID}}/tasks/{{.Task.ID}}/review"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="action" value="retry"><button>Approve retry</button></form>{{end}}</main></body></html>`))

func render(rw http.ResponseWriter) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
}

func (w *Web) Register(routes identity.Routes) {
	routes.Handle("GET /workspaces/{workspaceID}/tasks", w.Browser.Protect(false, w.list))
	routes.Handle("GET /workspaces/{workspaceID}/tasks/{taskID}", w.Browser.Protect(false, w.detail))
	routes.Handle("POST /workspaces/{workspaceID}/tasks/{taskID}/cancel", w.Browser.Protect(true, w.cancel))
	routes.Handle("POST /workspaces/{workspaceID}/tasks/{taskID}/review", w.Browser.Protect(true, w.review))
}

func (w *Web) list(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	items, err := w.Tasks.ListForParticipant(r.Context(), r.PathValue("workspaceID"), session.User.ID, r.URL.Query().Get("after"))
	if err != nil {
		http.Error(rw, "Task list unavailable", 403)
		return
	}
	next := ""
	if len(items) == 50 {
		next = items[len(items)-1].ID
	}
	render(rw)
	_ = listPage.Execute(rw, struct {
		WorkspaceID string
		Tasks       []tasks.ParticipantTask
		Next        string
	}{WorkspaceID: r.PathValue("workspaceID"), Tasks: items, Next: next})
}

func (w *Web) detail(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	task, err := w.Tasks.GetForParticipant(r.Context(), r.PathValue("workspaceID"), session.User.ID, r.PathValue("taskID"))
	if err != nil {
		http.Error(rw, "Task unavailable", 403)
		return
	}
	canCancel := task.FromOwnerUserID == session.User.ID && task.Status != "cancelled" && task.Status != "completed" && task.Status != "expired"
	canReview := (task.Status == "requires_approval" || task.Status == "interrupted" || task.Status == "failed") && (task.FromOwnerUserID == session.User.ID || task.ToOwnerUserID == session.User.ID)
	render(rw)
	_ = detailPage.Execute(rw, struct {
		WorkspaceID string
		CSRF        string
		Task        tasks.ParticipantTask
		CanCancel   bool
		CanReview   bool
	}{WorkspaceID: r.PathValue("workspaceID"), CSRF: session.CSRF, Task: task, CanCancel: canCancel, CanReview: canReview})
}

func (w *Web) cancel(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	task, err := w.Tasks.CancelForParticipant(r.Context(), r.PathValue("workspaceID"), session.User.ID, r.PathValue("taskID"))
	if err != nil {
		http.Error(rw, "Task cancellation denied", 403)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+task.WorkspaceID+"/tasks/"+task.ID, http.StatusSeeOther)
}

func (w *Web) review(rw http.ResponseWriter, r *http.Request, session identity.Session) {
	task, err := w.Tasks.ReviewForParticipant(r.Context(), r.PathValue("workspaceID"), session.User.ID, r.PathValue("taskID"), r.PostForm.Get("action"))
	if err != nil {
		http.Error(rw, "Task review denied", 403)
		return
	}
	http.Redirect(rw, r, "/workspaces/"+task.WorkspaceID+"/tasks/"+task.ID, http.StatusSeeOther)
}
