//go:build darwin || linux

package runner

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type stateLock struct{ file *os.File }

func lockState(dir string) (*stateLock, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("runner state directory unavailable")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("runner state directory must be owner-only and not a symlink")
	}
	f, err := os.OpenFile(filepath.Join(dir, "runner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("runner lock unavailable")
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another runner owns this state directory")
	}
	return &stateLock{f}, nil
}
func (l *stateLock) Close() error {
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return l.file.Close()
}
