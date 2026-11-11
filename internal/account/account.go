// Package account manages the users of the couple and their preferences (RF-01, RF-02).
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
)

// ErrInvalidCredentials is returned when the login or the password is wrong.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrNotFound is returned when the user does not exist.
var ErrNotFound = errors.New("user not found")

// Themes accepted as user preference (RNF-14).
var validThemes = map[string]bool{"system": true, "light": true, "dark": true}

// User is the public view of a user.
type User struct {
	ID      int64
	Login   string
	Name    string
	Theme   string
	Haptics bool
}

// Preferences holds optional preference changes; nil fields stay unchanged.
type Preferences struct {
	Theme   *string
	Haptics *bool
}

// Service creates, authenticates and updates users.
type Service struct {
	conn *sql.DB
	now  func() time.Time
	// dummyHash is verified when the login does not exist, so timing does not reveal valid logins.
	dummyHash string
}

// NewService creates the account service.
func NewService(conn *sql.DB) (*Service, error) {
	dummy, err := auth.HashPassword("radar-dummy-password")
	if err != nil {
		return nil, err
	}
	return &Service{conn: conn, now: time.Now, dummyHash: dummy}, nil
}

// Create adds a person of the couple and the user linked to it.
func (s *Service) Create(ctx context.Context, login, name, password string) (User, error) {
	login = strings.TrimSpace(login)
	name = strings.TrimSpace(name)
	if login == "" || name == "" {
		return User{}, errors.New("login e nome são obrigatórios")
	}
	if err := auth.ValidatePassword(password); err != nil {
		return User{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	var u store.User
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		p, err := q.CreatePerson(ctx, store.CreatePersonParams{Name: name, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("create person: %w", err)
		}
		u, err = q.CreateUser(ctx, store.CreateUserParams{
			Login: login, PasswordHash: hash, PersonID: p.ID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return User{ID: u.ID, Login: u.Login, Name: name, Theme: u.Theme, Haptics: u.Haptics}, nil
}

// Authenticate returns the user when login and password match.
func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error) {
	row, err := store.New(s.conn).GetUserByLogin(ctx, strings.TrimSpace(login))
	if errors.Is(err, sql.ErrNoRows) {
		_, _ = auth.VerifyPassword(s.dummyHash, password)
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	ok, err := auth.VerifyPassword(row.PasswordHash, password)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	return User{ID: row.ID, Login: row.Login, Name: row.PersonName, Theme: row.Theme, Haptics: row.Haptics}, nil
}

// Get returns the user with the given ID.
func (s *Service) Get(ctx context.Context, id int64) (User, error) {
	row, err := store.New(s.conn).GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Login: row.Login, Name: row.PersonName, Theme: row.Theme, Haptics: row.Haptics}, nil
}

// UpdatePreferences changes theme and haptic feedback for the user.
func (s *Service) UpdatePreferences(ctx context.Context, id int64, p Preferences) (User, error) {
	u, err := s.Get(ctx, id)
	if err != nil {
		return User{}, err
	}
	if p.Theme != nil {
		if !validThemes[*p.Theme] {
			return User{}, fmt.Errorf("tema inválido: %s", *p.Theme)
		}
		u.Theme = *p.Theme
	}
	if p.Haptics != nil {
		u.Haptics = *p.Haptics
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		return q.UpdateUserPreferences(ctx, store.UpdateUserPreferencesParams{
			Theme: u.Theme, Haptics: u.Haptics, UpdatedAt: s.now().UTC(), ID: id,
		})
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// SetPassword replaces the password of the user with the given login.
func (s *Service) SetPassword(ctx context.Context, login, password string) error {
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		n, err := q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{
			PasswordHash: hash, UpdatedAt: s.now().UTC(), Login: strings.TrimSpace(login),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// List returns every user.
func (s *Service) List(ctx context.Context) ([]store.ListUsersRow, error) {
	return store.New(s.conn).ListUsers(ctx)
}
