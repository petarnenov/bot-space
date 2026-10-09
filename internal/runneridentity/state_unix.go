//go:build darwin || linux

package runneridentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/petarnenov/bot-space/internal/security"
	"golang.org/x/sys/unix"
)

type stateProfile struct {
	Version int
	Origin  string
	Project string `json:"Project,omitempty"`
	Role    Role
	KeyHash string
}
type State struct {
	mu         sync.Mutex
	dir, lock  *os.File
	profile    stateProfile
	privateKey ed25519.PrivateKey
	local      *net.UnixListener
}

var ErrStateLocked = errors.New("runner state is already in use")
var ErrStateUnavailable = errors.New("private runner state unavailable")

func OpenState(path, origin string, role Role) (*State, error) {
	if !filepath.IsAbs(path) || !validRole(role) {
		return nil, ErrInvalid
	}
	api, err := NewClient(origin)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(path, 0700); err != nil {
		return nil, ErrStateUnavailable
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrStateUnavailable
	}
	dir := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != uint32(os.Getuid()) || stat.Mode&0077 != 0 {
		dir.Close()
		return nil, ErrStateUnavailable
	}
	s := &State{dir: dir}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	lock, err := s.open("runner.lock", unix.O_RDWR|unix.O_CREAT)
	if err != nil {
		return nil, err
	}
	s.lock = lock
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return nil, ErrStateLocked
	}
	seed, err := s.read("machine.seed", ed25519.SeedSize)
	if errors.Is(err, os.ErrNotExist) {
		seed = make([]byte, ed25519.SeedSize)
		if _, err = rand.Read(seed); err != nil {
			return nil, ErrStateUnavailable
		}
		if err = s.write("machine.seed", seed); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, ErrStateUnavailable
	}
	s.privateKey = ed25519.NewKeyFromSeed(seed)
	public := s.privateKey.Public().(ed25519.PublicKey)
	expected := stateProfile{Version: 2, Origin: api.origin, Role: role, KeyHash: security.Hash(string(public))}
	raw, err := s.read("profile.json", 8192)
	if errors.Is(err, os.ErrNotExist) {
		raw, _ = json.Marshal(expected)
		if err = s.write("profile.json", raw); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		var stored stateProfile
		if json.Unmarshal(raw, &stored) != nil {
			return nil, ErrInvalid
		}
		if stored.Version == 1 && stored.Origin == expected.Origin && stored.Role == expected.Role && stored.KeyHash == expected.KeyHash && security.ValidUUID(stored.Project) {
			// Preserve an older project-bound lease under its own scope.
			if old, err := s.read("credential.json", 256<<10); err == nil {
				var lease Lease
				if json.Unmarshal(old, &lease) != nil || lease.validate(false) != nil || !strings.EqualFold(lease.ProjectID, stored.Project) || lease.Role != stored.Role {
					return nil, ErrStateUnavailable
				}
				if err = s.write("credential-"+strings.ToLower(lease.ProjectID)+".json", old); err != nil {
					return nil, err
				}
				if unix.Unlinkat(int(s.dir.Fd()), "credential.json", 0) != nil || s.dir.Sync() != nil {
					return nil, ErrStateUnavailable
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			raw, _ = json.Marshal(expected)
			if err = s.write("profile.json", raw); err != nil {
				return nil, err
			}
		} else if stored != expected {
			return nil, ErrInvalid
		}
	}
	s.profile = expected
	ok = true
	return s, nil
}
func (s *State) open(name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(s.dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, ErrStateUnavailable
	}
	file := os.NewFile(uintptr(fd), name)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != uint32(os.Getuid()) || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 {
		file.Close()
		return nil, ErrStateUnavailable
	}
	return file, nil
}
func (s *State) read(name string, limit int) ([]byte, error) {
	f, err := s.open(name, unix.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil || len(raw) > limit {
		return nil, ErrStateUnavailable
	}
	return raw, nil
}
func (s *State) write(name string, raw []byte) error {
	// Inspect an existing destination without following links. Atomic rename
	// replaces the directory entry and never writes through a symlink target.
	var st unix.Stat_t
	err := unix.Fstatat(int(s.dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil && (st.Uid != uint32(os.Getuid()) || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600) {
		return ErrStateUnavailable
	}
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return ErrStateUnavailable
	}
	nonce, err := security.Secret()
	if err != nil {
		return ErrStateUnavailable
	}
	temp := ".write-" + nonce
	f, err := s.open(temp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(s.dir.Fd()), temp, 0)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrStateUnavailable
	}
	if unix.Renameat(int(s.dir.Fd()), temp, int(s.dir.Fd()), name) != nil || s.dir.Sync() != nil {
		return ErrStateUnavailable
	}
	return nil
}
func (s *State) SigningKey() ed25519.PrivateKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(ed25519.PrivateKey(nil), s.privateKey...)
}
func (s *State) SaveLease(lease Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return ErrStateUnavailable
	}
	if lease.Validate() != nil || lease.Role != s.profile.Role {
		return ErrInvalid
	}
	raw, err := json.Marshal(lease)
	if err != nil || len(raw) > 256<<10 {
		return ErrInvalid
	}
	return s.write("credential-"+strings.ToLower(lease.ProjectID)+".json", raw)
}
func (s *State) LoadLease(project string) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return Lease{}, ErrStateUnavailable
	}
	if !security.ValidUUID(project) {
		return Lease{}, ErrInvalid
	}
	raw, err := s.read("credential-"+strings.ToLower(project)+".json", 256<<10)
	if err != nil {
		return Lease{}, err
	}
	var lease Lease
	if json.Unmarshal(raw, &lease) != nil || lease.validate(false) != nil || !strings.EqualFold(lease.ProjectID, project) || lease.Role != s.profile.Role {
		return Lease{}, ErrStateUnavailable
	}
	return lease, nil
}
func (s *State) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.local != nil {
		s.local.Close()
		s.local = nil
	}
	for i := range s.privateKey {
		s.privateKey[i] = 0
	}
	s.privateKey = nil
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
	if s.dir != nil {
		err := s.dir.Close()
		s.dir = nil
		return err
	}
	return nil
}
