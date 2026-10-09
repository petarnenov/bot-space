//go:build darwin || linux

package runner

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func startProcess(ctx context.Context, cmd *exec.Cmd) (func(), error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	var once sync.Once
	stop := func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
			stop()
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}()
	return func() { stop(); once.Do(func() { close(done) }) }, nil
}
