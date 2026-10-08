// Package ratelimit provides bounded per-replica request admission.
package ratelimit

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type entry struct {
	tokens            float64
	updated, lastSeen time.Time
}
type Limiter struct {
	mu              sync.Mutex
	rate            float64
	burst, capacity int
	now             func() time.Time
	entries         map[string]*entry
	lastSweep       time.Time
}

func New(perMinute float64, burst, capacity int, clock func() time.Time) *Limiter {
	if perMinute <= 0 || burst < 1 || capacity < 1 {
		panic("invalid internal rate limit")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Limiter{rate: perMinute / 60, burst: burst, capacity: capacity, now: clock, entries: map[string]*entry{}}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= time.Minute {
		for key, e := range l.entries {
			if now.Sub(e.lastSeen) >= 10*time.Minute {
				delete(l.entries, key)
			}
		}
		l.lastSweep = now
	}
	e := l.entries[key]
	if e == nil {
		if len(l.entries) >= l.capacity {
			return false, time.Minute
		}
		e = &entry{tokens: float64(l.burst), updated: now, lastSeen: now}
		l.entries[key] = e
	}
	elapsed := now.Sub(e.updated).Seconds()
	if elapsed > 0 {
		e.tokens = math.Min(float64(l.burst), e.tokens+elapsed*l.rate)
	}
	e.updated = now
	e.lastSeen = now
	if e.tokens < 1 {
		return false, time.Duration(math.Ceil((1 - e.tokens) / l.rate * float64(time.Second)))
	}
	e.tokens--
	return true, 0
}

func Reject(rw http.ResponseWriter, wait time.Duration) {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 60 {
		seconds = 60
	}
	rw.Header().Set("Retry-After", strconv.Itoa(seconds))
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(http.StatusTooManyRequests)
	_, _ = rw.Write([]byte("{\"error\":{\"code\":\"rate_limited\",\"message\":\"Request rate exceeded\"}}\n"))
}

func PeerAdmission(login, mcp *Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var limiter *Limiter
		switch r.URL.Path {
		case "/auth/github/login":
			limiter = login
		case "/mcp":
			limiter = mcp
		}
		if limiter != nil {
			peer, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peer = r.RemoteAddr
			}
			if ok, wait := limiter.Allow(peer); !ok {
				Reject(rw, wait)
				return
			}
		}
		next.ServeHTTP(rw, r)
	})
}
