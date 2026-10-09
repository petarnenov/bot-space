package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/tasks"
)

type Supervisor struct {
	Config    Config
	mu        sync.Mutex
	jobs      map[string]*Job
	runnerID  string
	lock      *stateLock
	listener  net.Listener
	server    *http.Server
	clients   map[string]*mcpclient.Client
	ownership map[string]tasks.Ownership
	active    map[string]context.CancelFunc
	executor  func(context.Context, ProviderRequest) (ProviderResult, error)
}

type StartInput struct {
	AgentID        string `json:"agent_id"`
	Instruction    string `json:"instruction"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}
type JobStatus struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

func socketPath(state string) string {
	h := sha256.Sum256([]byte(state))
	return filepath.Join(os.TempDir(), "bs-"+hex.EncodeToString(h[:8]), "s")
}

func localClient(state string) *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath(state))
	}}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func LocalCall(ctx context.Context, state, method, path string, input, destination any) error {
	var body io.Reader
	if input != nil {
		reader, writer := io.Pipe()
		go func() { err := json.NewEncoder(writer).Encode(input); _ = writer.CloseWithError(err) }()
		body = reader
		defer reader.Close()
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://runner"+path, body)
	if err != nil {
		return errors.New("invalid local runner request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localClient(state).Do(req)
	if err != nil {
		return errors.New("local runner unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("local runner rejected request")
	}
	if destination != nil && json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(destination) != nil {
		return errors.New("invalid local runner response")
	}
	return nil
}

func randomID() (string, error) {
	secret, err := security.Secret()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(secret))
	h[6] = (h[6] & 15) | 64
	h[8] = (h[8] & 63) | 128
	x := hex.EncodeToString(h[:16])
	return x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:], nil
}

func NewSupervisor(config Config) (*Supervisor, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	lock, err := lockState(config.StateDir)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{Config: config, jobs: map[string]*Job{}, lock: lock, clients: map[string]*mcpclient.Client{}, ownership: map[string]tasks.Ownership{}, active: map[string]context.CancelFunc{}, executor: RunProvider}
	ok := false
	defer func() {
		if !ok {
			lock.Close()
		}
	}()
	if err = os.MkdirAll(filepath.Join(config.StateDir, "jobs"), 0700); err != nil {
		return nil, errors.New("runner journal directory unavailable")
	}
	var identity struct {
		ID string `json:"id"`
	}
	path := filepath.Join(config.StateDir, "identity.json")
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		identity.ID, err = randomID()
		if err != nil {
			return nil, err
		}
		if err = atomicJSON(path, identity); err != nil {
			return nil, err
		}
	} else if err = readJSON(path, &identity); err != nil {
		return nil, err
	}
	if !security.ValidUUID(identity.ID) {
		return nil, errors.New("invalid runner identity")
	}
	s.runnerID = identity.ID
	jobs, err := loadJobs(config.StateDir)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		s.jobs[jobs[i].ID] = &jobs[i]
	}
	ok = true
	return s, nil
}

func (s *Supervisor) agent(id string) (AgentConfig, bool) {
	for _, a := range s.Config.Agents {
		if a.ID == id {
			return a, true
		}
	}
	return AgentConfig{}, false
}

func (s *Supervisor) Start(input StartInput) (JobStatus, error) {
	if _, ok := s.agent(input.AgentID); !ok {
		return JobStatus{}, errors.New("agent not configured on this machine")
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = tasks.DefaultTimeoutSeconds
	}
	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > tasks.MaxTimeoutSeconds || input.Instruction == "" || len(input.Instruction) > tasks.MaxInstructionBytes {
		return JobStatus{}, errors.New("invalid local task input")
	}
	id, err := randomID()
	if err != nil {
		return JobStatus{}, err
	}
	capability, err := security.Secret()
	if err != nil {
		return JobStatus{}, err
	}
	j := &Job{ID: id, AgentID: input.AgentID, Instruction: input.Instruction, Status: "queued", Deadline: time.Now().Add(time.Duration(input.TimeoutSeconds) * time.Second), Capability: capability}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = saveJob(s.Config.StateDir, j); err != nil {
		return JobStatus{}, err
	}
	s.jobs[id] = j
	return status(j), nil
}

func status(j *Job) JobStatus {
	return JobStatus{ID: j.ID, AgentID: j.AgentID, Status: j.Status, SessionID: j.SessionID, ErrorCode: j.ErrorCode}
}

func localJSON(rw http.ResponseWriter, value any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(rw).Encode(value)
}

func readLocal(rw http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(rw, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(destination) != nil || d.Decode(&struct{}{}) != io.EOF {
		http.Error(rw, "Invalid local request", 400)
		return false
	}
	return true
}

func (s *Supervisor) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /start", func(rw http.ResponseWriter, r *http.Request) {
		var input StartInput
		if !readLocal(rw, r, &input) {
			return
		}
		result, err := s.Start(input)
		if err != nil {
			http.Error(rw, "Task creation rejected", 400)
			return
		}
		localJSON(rw, result)
	})
	mux.HandleFunc("GET /status", func(rw http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []JobStatus{}
		for _, j := range s.jobs {
			out = append(out, status(j))
		}
		localJSON(rw, out)
	})
	mux.HandleFunc("GET /jobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		j := s.jobs[r.PathValue("id")]
		if j == nil {
			http.Error(rw, "Job unavailable", 404)
			return
		}
		localJSON(rw, struct {
			JobStatus
			Result string `json:"result,omitempty"`
		}{status(j), j.Result})
	})
	mux.HandleFunc("POST /jobs/{id}/delegate", s.delegateHTTP)
	mux.HandleFunc("POST /jobs/{id}/identity", s.identityHTTP)
	mux.HandleFunc("POST /jobs/{id}/directory", s.directoryHTTP)
	return mux
}

func (s *Supervisor) listen() error {
	path := socketPath(s.Config.StateDir)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("local control directory unavailable")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("local control directory must be private")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.New("stale local socket cannot be removed")
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return errors.New("local control listener unavailable")
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		return errors.New("local control socket permission failure")
	}
	s.listener = listener
	s.server = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	return nil
}

func (s *Supervisor) Close() error {
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
		_ = os.Remove(socketPath(s.Config.StateDir))
	}
	return s.lock.Close()
}
