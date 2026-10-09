//go:build darwin || linux

package runneridentity

import (
	"github.com/petarnenov/bot-space/internal/security"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// ListenLocal is owned by the state lock; future bridge operations additionally
// require a role/task capability. A socket is never a global credential API.
func (s *State) ListenLocal() (*net.UnixListener, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil || s.local != nil {
		return nil, ErrStateUnavailable
	}
	canonical, err := filepath.EvalSymlinks(s.dir.Name())
	if err != nil {
		return nil, ErrStateUnavailable
	}
	dir := filepath.Join("/tmp", "tf-"+strconv.Itoa(os.Getuid())+"-"+security.Hash(canonical)[:16])
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return nil, ErrStateUnavailable
	}
	var st unix.Stat_t
	if unix.Lstat(dir, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Getuid()) || st.Mode&0077 != 0 {
		return nil, ErrStateUnavailable
	}
	path := filepath.Join(dir, "control.sock")
	if err = unix.Lstat(path, &st); err == nil {
		if st.Mode&unix.S_IFMT != unix.S_IFSOCK || st.Uid != uint32(os.Getuid()) || st.Mode&0077 != 0 {
			return nil, ErrStateUnavailable
		}
		// The exclusive state lock proves no earlier owner of this state remains.
		if os.Remove(path) != nil {
			return nil, ErrStateUnavailable
		}
	} else if !os.IsNotExist(err) {
		return nil, ErrStateUnavailable
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, ErrStateUnavailable
	}
	if os.Chmod(path, 0600) != nil {
		listener.Close()
		return nil, ErrStateUnavailable
	}
	s.local = listener
	return listener, nil
}
