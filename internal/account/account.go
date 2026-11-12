// Package account manages people, their users, PINs and preferences, and the groups they belong to (RF-01, RF-02, RF-78 to RF-81).
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
)

// ErrNotFound is returned when the user does not exist.
var ErrNotFound = errors.New("user not found")

// Themes accepted as user preference (RNF-14).
var validThemes = map[string]bool{"system": true, "light": true, "dark": true}

// User is the public view of a user.
type User struct {
	ID       int64
	PersonID int64
	Login    string
	Name     string
	Theme    string
	Haptics  bool
}

// Preferences holds optional preference changes; nil fields stay unchanged.
type Preferences struct {
	Theme   *string
	Haptics *bool
}

// Service creates, finds and updates users.
type Service struct {
	conn *sql.DB
	now  func() time.Time
}

// NewService creates the account service.
func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn, now: time.Now}
}

// Create adds a person and the user linked to it, without a group or a PIN yet (RF-80).
func (s *Service) Create(ctx context.Context, login, name string) (User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	name = strings.TrimSpace(name)
	if login == "" || name == "" {
		return User{}, errors.New("login e nome são obrigatórios")
	}
	now := s.now().UTC()
	var u store.User
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		p, err := q.CreatePerson(ctx, store.CreatePersonParams{Name: name, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("create person: %w", err)
		}
		u, err = q.CreateUser(ctx, store.CreateUserParams{Login: login, PersonID: p.ID, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return User{ID: u.ID, PersonID: u.PersonID, Login: u.Login, Name: name, Theme: u.Theme, Haptics: u.Haptics}, nil
}

// GetByLogin returns the user with the given login.
func (s *Service) GetByLogin(ctx context.Context, login string) (User, error) {
	row, err := store.New(s.conn).GetUserByLogin(ctx, strings.TrimSpace(login))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, PersonID: row.PersonID, Login: row.Login, Name: row.PersonName, Theme: row.Theme, Haptics: row.Haptics}, nil
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
	return User{ID: row.ID, PersonID: row.PersonID, Login: row.Login, Name: row.PersonName, Theme: row.Theme, Haptics: row.Haptics}, nil
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

// List returns every user, in creation order.
func (s *Service) List(ctx context.Context) ([]User, error) {
	rows, err := store.New(s.conn).ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(rows))
	for _, r := range rows {
		out = append(out, User{ID: r.ID, Login: r.Login, Name: r.PersonName})
	}
	return out, nil
}

// Person is one person of the couple, holder of accounts, cards and incomes.
type Person struct {
	ID   int64
	Name string
}

// ListPersons returns the people of the couple in creation order.
func (s *Service) ListPersons(ctx context.Context) ([]Person, error) {
	rows, err := store.New(s.conn).ListPersons(ctx)
	if err != nil {
		return nil, fmt.Errorf("list persons: %w", err)
	}
	out := make([]Person, 0, len(rows))
	for _, r := range rows {
		out = append(out, Person{ID: r.ID, Name: r.Name})
	}
	return out, nil
}
