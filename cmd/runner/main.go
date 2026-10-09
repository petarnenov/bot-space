//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

type projects []string

func (p *projects) String() string { return "project UUIDs" }
func (p *projects) Set(value string) error {
	if !security.ValidUUID(value) {
		return errors.New("project must be a UUID")
	}
	*p = append(*p, value)
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, openBrowser); err != nil {
		fmt.Fprintln(os.Stderr, "Runner command failed:", err)
		os.Exit(1)
	}
}

// The full serve/control/provider runtime remains tracked separately. Enroll is
// a concrete authentication entrypoint reused by automatic startup integration.
func run(ctx context.Context, args []string, out io.Writer, open func(string) error) error {
	if len(args) == 0 || (args[0] != "enroll" && args[0] != "doctor" && args[0] != "serve") {
		return errors.New("usage: runner serve|enroll|doctor --server ORIGIN --state ABSOLUTE_PATH --role architect|executor --project UUID [--project UUID]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	server := flags.String("server", "", "The Firm HTTPS origin")
	stateDir := flags.String("state", "", "Private absolute role state directory")
	role := flags.String("role", "", "architect or executor")
	noOpen := flags.Bool("no-open", false, "Print verified GitHub sign-in URL for a headless machine")
	var scopes projects
	flags.Var(&scopes, "project", "Eligible project UUID; repeat for multiple projects")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid runner arguments")
	}
	if flags.NArg() != 0 || len(scopes) == 0 || len(scopes) > 128 {
		return errors.New("configure 1 to 128 eligible projects")
	}
	if *role != "architect" && *role != "executor" {
		return errors.New("role must be architect or executor")
	}
	api, err := runneridentity.NewClient(*server)
	if err != nil {
		return err
	}
	state, err := runneridentity.OpenState(*stateDir, *server, runneridentity.Role(*role))
	if err != nil {
		return err
	}
	defer state.Close()
	session, err := runneridentity.NewSession(state, api)
	if err != nil {
		return err
	}
	if *noOpen {
		open = func(url string) error {
			_, err := fmt.Fprintln(out, "Open GitHub sign-in:", url)
			if err != nil {
				return errors.New("login URL output unavailable")
			}
			return nil
		}
	}
	if args[0] == "serve" {
		if _, err := state.ListenLocal(); err != nil {
			return err
		}
		return serveIdentity(ctx, session, scopes, open, out, func(ctx context.Context, lease runneridentity.Lease) (io.Closer, error) {
			return connectDelivery(ctx, lease, state)
		})
	}
	seen := map[string]bool{}
	for _, project := range scopes {
		project = strings.ToLower(project)
		if seen[project] {
			continue
		}
		seen[project] = true
		var lease runneridentity.Lease
		if args[0] == "doctor" {
			lease, err = session.Refresh(ctx, project)
		} else {
			lease, err = session.Acquire(ctx, project, open)
		}
		if err != nil {
			return err
		}
		if args[0] == "doctor" {
			connection, native, err := runneridentity.DialNative(lease)
			if err != nil {
				return err
			}
			check, cancel := context.WithTimeout(ctx, 5*time.Second)
			identity, err := native.Inspect(check, &pb.InspectRequest{ResourceId: lease.RunnerID})
			cancel()
			connection.Close()
			if err != nil || identity.GetResourceId() != lease.RunnerID || identity.GetState() != "enrolled" {
				return errors.New("native identity verification failed")
			}
		}
		// Never encode the lease: it contains a private bearer token.
		if err = json.NewEncoder(out).Encode(struct {
			RunnerID  string              `json:"runner_id"`
			ProjectID string              `json:"project_id"`
			Role      runneridentity.Role `json:"role"`
			Epoch     uint64              `json:"epoch"`
		}{lease.RunnerID, lease.ProjectID, lease.Role, lease.Epoch}); err != nil {
			return errors.New("runner status output unavailable")
		}
	}
	return nil
}
func openBrowser(url string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "/usr/bin/open"
	}
	// Argument arrays preserve the URL without invoking a shell.
	child := exec.Command(command, url)
	child.Stdout = io.Discard
	child.Stderr = io.Discard
	if err := child.Start(); err != nil {
		return errors.New("GitHub login browser unavailable")
	}
	go child.Wait()
	return nil
}
