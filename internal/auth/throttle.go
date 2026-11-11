package auth

import (
	"strings"
	"sync"
	"time"
)

// Throttle blocks a login for a while after too many failed attempts.
type Throttle struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	lock    time.Duration
	now     func() time.Time
	entries map[string]*throttleEntry
}

// throttleEntry tracks the failures of one login.
type throttleEntry struct {
	failures    int
	first       time.Time
	lockedUntil time.Time
}

// NewThrottle creates a throttle that locks for lock after max failures within window.
func NewThrottle(maxFailures int, window, lock time.Duration, now func() time.Time) *Throttle {
	if now == nil {
		now = time.Now
	}
	return &Throttle{max: maxFailures, window: window, lock: lock, now: now, entries: map[string]*throttleEntry{}}
}

// Allowed reports whether the login may try again and, if not, how long to wait.
func (t *Throttle) Allowed(login string) (bool, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[normalizeLogin(login)]
	if !ok {
		return true, 0
	}
	if wait := e.lockedUntil.Sub(t.now()); wait > 0 {
		return false, wait
	}
	return true, 0
}

// Failure records a failed attempt and locks the login when the limit is reached.
func (t *Throttle) Failure(login string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	key := normalizeLogin(login)
	e, ok := t.entries[key]
	if !ok || now.Sub(e.first) > t.window {
		e = &throttleEntry{first: now, lockedUntil: timeOrZero(e)}
		t.entries[key] = e
	}
	e.failures++
	if e.failures >= t.max {
		e.lockedUntil = now.Add(t.lock)
		e.failures = 0
		e.first = now
	}
}

// Success clears the failures of the login.
func (t *Throttle) Success(login string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, normalizeLogin(login))
}

// timeOrZero keeps an existing lock when an entry is renewed.
func timeOrZero(e *throttleEntry) time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.lockedUntil
}

// normalizeLogin makes logins case insensitive, matching the database collation.
func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}
