//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/runneridentity"
)

func TestSignInURLPreservesTargetAndVisibleURL(t *testing.T) {
	url := "https://example.com/auth/github/login?return_to=%2Frunners%2Fenroll%2F11111111-1111-4111-8111-111111111111&extra=" + strings.Repeat("x", 160)
	for _, hyperlink := range []bool{false, true} {
		var out bytes.Buffer
		if err := printSignInURL(&out, url, hyperlink); err != nil {
			t.Fatal(err)
		}
		want := "Open GitHub sign-in:\n" + url + "\n"
		if hyperlink {
			want = "Open GitHub sign-in (Cmd+click):\n\x1b]8;;" + url + "\x1b\\" + url + "\x1b]8;;\x1b\\\n"
		}
		if out.String() != want {
			t.Fatalf("hyperlink=%v: unexpected output %q", hyperlink, out.String())
		}
	}
}

type failedLoginWriter struct{ err error }

func (w failedLoginWriter) Write([]byte) (int, error) { return 0, w.err }

func TestSignInURLReportsWriteFailure(t *testing.T) {
	want := errors.New("output closed")
	for _, hyperlink := range []bool{false, true} {
		if err := printSignInURL(failedLoginWriter{want}, "https://example.com", hyperlink); !errors.Is(err, want) {
			t.Fatalf("hyperlink=%v: got %v, want write error", hyperlink, err)
		}
	}
}

func TestTerminalHyperlinksRejectRedirectedOutput(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	for _, out := range []*os.File{file, write} {
		if terminalHyperlinks(out) {
			t.Fatal("redirected output enables hyperlinks")
		}
	}
	if terminalHyperlinks(&bytes.Buffer{}) {
		t.Fatal("buffer enables hyperlinks")
	}
}

func TestTerminalHyperlinksForInteractiveOutput(t *testing.T) {
	terminal, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		t.Skip("requires a controlling terminal")
	}
	defer terminal.Close()
	t.Setenv("TERM", "xterm-256color")
	if !terminalHyperlinks(terminal) {
		t.Fatal("interactive output does not enable hyperlinks")
	}
	t.Setenv("TERM", "dumb")
	if terminalHyperlinks(terminal) {
		t.Fatal("dumb terminal enables hyperlinks")
	}
}

type cancelLoginWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelLoginWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	w.cancel()
	return n, err
}

func TestNoOpenEnrollmentPrintsOnlyVerifiedURLForBothRoles(t *testing.T) {
	for _, role := range []string{"architect", "executor"} {
		for _, trusted := range []bool{true, false} {
			name := role + "/verified"
			if !trusted {
				name = role + "/untrusted"
			}
			t.Run(name, func(t *testing.T) {
				const project = "11111111-1111-4111-8111-111111111111"
				const enrollment = "22222222-2222-4222-8222-222222222222"
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/runners/enroll" {
						http.Error(w, "unexpected request", http.StatusBadRequest)
						return
					}
					var proof runneridentity.StartProof
					if err := json.NewDecoder(r.Body).Decode(&proof); err != nil {
						t.Error(err)
						return
					}
					if err := runneridentity.VerifyStart(proof); err != nil {
						t.Error(err)
						return
					}
					if proof.ProjectID != project || string(proof.Role) != role {
						t.Error("incorrect enrollment scope or role")
					}
					url := server.URL + "/auth/github/login?return_to=" + runneridentity.EnrollmentPath + enrollment
					if !trusted {
						url = "https://attacker.example/auth"
					}
					json.NewEncoder(w).Encode(map[string]any{
						"id": enrollment, "project_id": project, "role": role,
						"expires_at": time.Now().Add(time.Minute), "login_url": url,
					})
				}))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				out := &cancelLoginWriter{cancel: cancel}
				opened := false
				err := run(ctx, []string{
					"enroll", "--server", server.URL, "--state", filepath.Join(t.TempDir(), role),
					"--role", role, "--project", project, "--no-open",
				}, out, func(string) error { opened = true; return nil })
				if err == nil {
					t.Fatal("enrollment should stop before claiming credentials")
				}
				if opened {
					t.Fatal("--no-open launched the browser")
				}
				want := ""
				if trusted {
					want = "Open GitHub sign-in:\n" + server.URL + "/auth/github/login?return_to=" + runneridentity.EnrollmentPath + enrollment + "\n"
				}
				if out.String() != want {
					t.Fatalf("got %q, want %q", out.String(), want)
				}
			})
		}
	}
}
