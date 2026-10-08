package httpserver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestShutdownHelper(t *testing.T) {
	if os.Getenv("BOT_SPACE_SHUTDOWN_HELPER") != "1" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	s := New(func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.HTTP.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("started")
		if os.Getenv("BOT_SPACE_SHUTDOWN_MODE") == "complete" {
			select {
			case <-time.After(2 * time.Second):
				_, _ = io.WriteString(w, "completed")
			case <-r.Context().Done():
			}
		} else {
			<-r.Context().Done()
		}
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	fmt.Println(listener.Addr().String())
	if s.Serve(ctx, listener) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestSignalShutdown(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		signal os.Signal
	}{{"complete", syscall.SIGINT}, {"deadline", syscall.SIGTERM}} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownHelper$")
			cmd.Env = append(os.Environ(), "BOT_SPACE_SHUTDOWN_HELPER=1", "BOT_SPACE_SHUTDOWN_MODE="+tc.mode)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() {
				t.Fatal("helper not listening")
			}
			conn, err := net.DialTimeout("tcp", scanner.Text(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(24 * time.Second))
			_, _ = io.WriteString(conn, "GET /slow HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
			if !scanner.Scan() || scanner.Text() != "started" {
				t.Fatal("helper handler not started")
			}
			start := time.Now()
			if err := cmd.Process.Signal(tc.signal); err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(conn)
			if err := cmd.Wait(); err != nil {
				t.Fatalf("shutdown failed: %v", err)
			}
			if tc.mode == "complete" {
				if readErr != nil || !strings.Contains(string(data), "completed") {
					t.Fatal("in-flight request did not complete")
				}
			} else if elapsed := time.Since(start); elapsed < DrainTimeout-time.Second || elapsed > DrainTimeout+3*time.Second {
				t.Fatalf("shutdown deadline: %s", elapsed)
			}
		})
	}
}
