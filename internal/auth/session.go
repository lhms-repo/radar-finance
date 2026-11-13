// Package auth handles sessions and the logged in user (RF-01, RF-03, RNF-10).
package auth

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/scope"
)

// sessionUserKey is the session key that stores the logged in user ID.
const sessionUserKey = "userID"

// sessionGroupKey is the session key that stores the active group (RF-81).
const sessionGroupKey = "groupID"

// ctxKey is the private type for values this package stores in a context.
type ctxKey int

// userIDKey is the context key of the authenticated user ID (RF-03).
const userIDKey ctxKey = 1

// SessionOptions configures the session manager.
type SessionOptions struct {
	Idle     time.Duration
	Lifetime time.Duration
	Secure   bool
}

// NewSessionManager creates the scs manager backed by the SQLite sessions table (RNF-10).
func NewSessionManager(conn *sql.DB, opts SessionOptions) *scs.SessionManager {
	sm := scs.New()
	sm.Store = &SQLiteStore{conn: conn}
	fallback := sm.ErrorFunc
	// A late commit of a signed out session is dropped without sending its dead cookie back.
	sm.ErrorFunc = func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, errRevoked) {
			return
		}
		fallback(w, r, err)
	}
	sm.IdleTimeout = opts.Idle
	sm.Lifetime = opts.Lifetime
	sm.Cookie.Name = "radar_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = opts.Secure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Persist = true
	return sm
}

// SignIn renews the session token and binds the session to the user and the group chosen at the entry (RF-01).
func SignIn(ctx context.Context, sm *scs.SessionManager, userID, groupID int64) error {
	if err := sm.RenewToken(ctx); err != nil {
		return err
	}
	sm.Put(ctx, sessionUserKey, userID)
	sm.Put(ctx, sessionGroupKey, groupID)
	return nil
}

// SwitchGroup changes the active group of the session; the caller checks the membership first (RF-81).
// The group is also written beside the session data, where a request that loaded the session before the switch
// and saves it after cannot bring the old group back.
func SwitchGroup(ctx context.Context, sm *scs.SessionManager, groupID int64) error {
	sm.Put(ctx, sessionGroupKey, groupID)
	st, ok := sm.Store.(*SQLiteStore)
	if !ok || sm.Token(ctx) == "" {
		return nil
	}
	return db.WithTx(ctx, st.conn, func(q *store.Queries) error {
		return q.SetSessionGroup(ctx, store.SetSessionGroupParams{GroupID: sql.NullInt64{Int64: groupID, Valid: true}, Token: sm.Token(ctx)})
	})
}

// activeGroup returns the group of a session loaded in ctx: the one of the last switch, or the one chosen at the entry.
func activeGroup(ctx context.Context, sm *scs.SessionManager) int64 {
	if st, ok := sm.Store.(*SQLiteStore); ok && sm.Token(ctx) != "" {
		if g, err := store.New(st.conn).SessionGroup(ctx, sm.Token(ctx)); err == nil && g > 0 {
			return g
		}
	}
	return sm.GetInt64(ctx, sessionGroupKey)
}

// SignOut destroys the current session.
func SignOut(ctx context.Context, sm *scs.SessionManager) error {
	return sm.Destroy(ctx)
}

// RequireUser rejects requests without a logged in user and an active group, except the public ones, and puts
// both in the request context. A session from before groups has no group and goes back to the entry.
func RequireUser(sm *scs.SessionManager, public func(*http.Request) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r) {
			next.ServeHTTP(w, r)
			return
		}
		id := sm.GetInt64(r.Context(), sessionUserKey)
		group := activeGroup(r.Context(), sm)
		if id == 0 || group == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"Sua sessão expirou. Entre novamente."}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(scope.WithGroup(WithUserID(r.Context(), id), group)))
	})
}

// WithUserID returns a context carrying the authenticated user ID.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserID returns the authenticated user ID stored in the context, or zero.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}

// revokedFor is how long a deleted token stays refused, far longer than any request that loaded it.
const revokedFor = time.Hour

// errRevoked stops scs from saving a session deleted by a logout and from writing its cookie.
var errRevoked = errors.New("session token was revoked")

// SQLiteStore implements scs.Store and scs.CtxStore on the sessions table.
//
// With an idle timeout, scs saves the session at the end of every request. A request that loaded the
// session before a logout and ends after it would write the session back, so deleted tokens are kept
// for a while and their late commits are dropped (RF-01).
type SQLiteStore struct {
	conn *sql.DB
	// gate lets commits run together but never across a delete, so a commit that passed the
	// revoked check cannot land after the delete of its token.
	gate    sync.RWMutex
	mu      sync.Mutex
	revoked map[string]time.Time
}

// isRevoked tells whether the token was deleted recently, forgetting entries past revokedFor.
func (s *SQLiteStore) isRevoked(token string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for t, at := range s.revoked {
		if now.Sub(at) > revokedFor {
			delete(s.revoked, t)
		}
	}
	_, ok := s.revoked[token]
	return ok
}

// revoke remembers a deleted token so a late commit cannot bring it back.
func (s *SQLiteStore) revoke(token string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked == nil {
		s.revoked = map[string]time.Time{}
	}
	s.revoked[token] = now
}

// Find returns the session data for a valid, unexpired token.
func (s *SQLiteStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

// FindCtx returns the session data for a valid, unexpired token.
func (s *SQLiteStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	data, err := store.New(s.conn).FindSession(ctx, store.FindSessionParams{Token: token, ExpiryMs: time.Now().UnixMilli()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Commit saves the session data with its expiry.
func (s *SQLiteStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}

// CommitCtx saves the session data with its expiry; a token deleted by a logout stays deleted.
func (s *SQLiteStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	s.gate.RLock()
	defer s.gate.RUnlock()
	if s.isRevoked(token, time.Now()) {
		return errRevoked
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		return q.UpsertSession(ctx, store.UpsertSessionParams{Token: token, Data: b, ExpiryMs: expiry.UnixMilli()})
	})
}

// Delete removes the session.
func (s *SQLiteStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

// DeleteCtx removes the session and refuses later commits of its token.
func (s *SQLiteStore) DeleteCtx(ctx context.Context, token string) error {
	s.gate.Lock()
	defer s.gate.Unlock()
	s.revoke(token, time.Now())
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		return q.DeleteSession(ctx, token)
	})
}

// CleanupSessions deletes expired sessions every interval until ctx is done.
func CleanupSessions(ctx context.Context, conn *sql.DB, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := db.WithTx(ctx, conn, func(q *store.Queries) error {
				n, err := q.DeleteExpiredSessions(ctx, time.Now().UnixMilli())
				if n > 0 {
					log.Info("expired sessions removed", "count", n)
				}
				return err
			})
			if err != nil && ctx.Err() == nil {
				log.Error("cleanup sessions", "err", err)
			}
		}
	}
}

// SessionUser reads the user and the active group of the session cookie directly, for long lived streams that
// cannot pass through the buffering session middleware; zeros when there is no valid session.
func SessionUser(sm *scs.SessionManager, r *http.Request) (userID, groupID int64) {
	c, err := r.Cookie(sm.Cookie.Name)
	if err != nil || c.Value == "" {
		return 0, 0
	}
	ctx, err := sm.Load(r.Context(), c.Value)
	if err != nil {
		return 0, 0
	}
	return sm.GetInt64(ctx, sessionUserKey), activeGroup(ctx, sm)
}

// CurrentUser returns the user of a session already loaded in the context, zero when signed out.
func CurrentUser(ctx context.Context, sm *scs.SessionManager) int64 {
	return sm.GetInt64(ctx, sessionUserKey)
}

// CurrentGroup returns the active group of a session already loaded in the context, zero when there is none.
func CurrentGroup(ctx context.Context, sm *scs.SessionManager) int64 {
	return activeGroup(ctx, sm)
}
