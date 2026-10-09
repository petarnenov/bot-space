// Package repositoryaccess verifies immutable GitHub repository identities and
// explicit collaborator membership. Public visibility never grants admission.
package repositoryaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxCacheAge = 60 * time.Second
const RequestTimeout = 5 * time.Second
const BodyLimit = 1 << 20
const MaxPages = 100

var ErrDenied = errors.New("repository collaborator access required")
var ErrUnavailable = errors.New("repository authority unavailable")
var ErrInvalid = errors.New("invalid repository identity")
var part = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

type Repository struct {
	ID, OwnerID int64
	Owner, Name string
}
type TokenSource func(context.Context) (string, error)
type cacheKey struct {
	Repository
	UserID int64
}
type cached struct {
	Until   time.Time
	Allowed bool
	Seq     uint64
}
type Checker struct {
	token    TokenSource
	client   *http.Client
	now      func() time.Time
	mu       sync.Mutex
	cache    map[cacheKey]cached
	sequence uint64
}

func New(token TokenSource) (*Checker, error) {
	if token == nil {
		return nil, ErrInvalid
	}
	return &Checker{token: token, client: &http.Client{Timeout: RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now, cache: make(map[cacheKey]cached)}, nil
}

// Verify uses only a current, bounded cache. force bypasses cached admission for
// startup/credential refresh. UserID comes from authenticated GitHub identity.
func (c *Checker) Verify(ctx context.Context, repo Repository, userID int64, force bool) error {
	if repo.ID < 1 || repo.OwnerID < 1 || userID < 1 || !part.MatchString(repo.Owner) || !part.MatchString(repo.Name) {
		return ErrInvalid
	}
	repo.Owner = strings.ToLower(repo.Owner)
	repo.Name = strings.ToLower(repo.Name)
	key := cacheKey{repo, userID}
	start := c.now()
	c.mu.Lock()
	entry, ok := c.cache[key]
	if !force && ok && start.Before(entry.Until) {
		c.mu.Unlock()
		if entry.Allowed {
			return nil
		}
		return ErrDenied
	}
	if len(c.cache) >= 10000 {
		c.cache = make(map[cacheKey]cached)
	}
	c.sequence++
	sequence := c.sequence
	c.cache[key] = cached{Seq: sequence}
	c.mu.Unlock()
	check, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	token, err := c.token(check)
	allowed := false
	if err != nil || token == "" || len(token) > 4096 || strings.ContainsAny(token, " \t\r\n") {
		err = ErrUnavailable
	} else {
		allowed, err = c.check(check, repo, userID, token)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache[key].Seq != sequence {
		return ErrUnavailable
	}
	if err != nil {
		delete(c.cache, key)
		return err
	}
	// An older concurrent verification cannot resurrect removed authority.
	c.cache[key] = cached{Until: start.Add(MaxCacheAge), Allowed: allowed, Seq: sequence}
	if !allowed {
		return ErrDenied
	}
	return nil
}

func (c *Checker) get(ctx context.Context, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := c.client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, BodyLimit+1))
	if err != nil || len(body) > BodyLimit {
		return ErrUnavailable
	}
	if json.Unmarshal(body, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (c *Checker) check(ctx context.Context, repo Repository, userID int64, token string) (bool, error) {
	base := "/repos/" + repo.Owner + "/" + repo.Name
	var details struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Owner struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := c.get(ctx, base, token, &details); err != nil {
		return false, err
	}
	if details.ID != repo.ID || details.Owner.ID != repo.OwnerID || !strings.EqualFold(details.Name, repo.Name) || !strings.EqualFold(details.Owner.Login, repo.Owner) {
		return false, ErrUnavailable
	}
	if details.Owner.ID == userID {
		return true, nil
	}
	// Filter direct grants, not general read permission on a public repository.
	for page := 1; page <= MaxPages; page++ {
		var members []struct {
			ID int64 `json:"id"`
		}
		if err := c.get(ctx, fmt.Sprintf("%s/collaborators?affiliation=direct&per_page=100&page=%d", base, page), token, &members); err != nil {
			return false, err
		}
		if len(members) > 100 {
			return false, ErrUnavailable
		}
		for _, m := range members {
			if m.ID < 1 {
				return false, ErrUnavailable
			}
			if m.ID == userID {
				return true, nil
			}
		}
		if len(members) < 100 {
			return false, nil
		}
	}
	return false, ErrUnavailable
}
