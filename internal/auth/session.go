package auth

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
)

// sessionUserKey is the session key that stores the logged in user ID.
const sessionUserKey = "userID"

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
	sm.IdleTimeout = opts.Idle
	sm.Lifetime = opts.Lifetime
	sm.Cookie.Name = "radar_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = opts.Secure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Persist = true
	return sm
}

// SignIn renews the session token and binds the session to the user.
func SignIn(ctx context.Context, sm *scs.SessionManager, userID int64) error {
	if err := sm.RenewToken(ctx); err != nil {
		return err
	}
	sm.Put(ctx, sessionUserKey, userID)
	return nil
}

// SignOut destroys the current session.
func SignOut(ctx context.Context, sm *scs.SessionManager) error {
	return sm.Destroy(ctx)
}

// RequireUser rejects requests without a logged in user, except the public paths.
func RequireUser(sm *scs.SessionManager, public map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		id := sm.GetInt64(r.Context(), sessionUserKey)
		if id == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"Sua sessão expirou. Entre novamente."}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), id)))
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

// SQLiteStore implements scs.Store and scs.CtxStore on the sessions table.
type SQLiteStore struct {
	conn *sql.DB
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

// CommitCtx saves the session data with its expiry.
func (s *SQLiteStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		return q.UpsertSession(ctx, store.UpsertSessionParams{Token: token, Data: b, ExpiryMs: expiry.UnixMilli()})
	})
}

// Delete removes the session.
func (s *SQLiteStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

// DeleteCtx removes the session.
func (s *SQLiteStore) DeleteCtx(ctx context.Context, token string) error {
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
