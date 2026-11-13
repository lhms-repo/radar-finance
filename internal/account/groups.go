package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// Role of a member in a group (RF-79).
type Role string

// Member roles: administrators manage the group and its people (RF-80).
// kindPersonal marks the personal space of one person in groups.kind (RF-83).
const kindPersonal = "personal"

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Limits of the PIN lockout (RN-27).
const (
	maxPINFailures = 5
	pinLockout     = 15 * time.Minute
	maxGroupName   = 40
	fullWeightBP   = 10_000
)

// Business errors of groups and of entering with a PIN.
var (
	ErrGroupNotFound      = problem.NotFound("group_not_found", "Grupo não encontrado.")
	ErrGroupNameRequired  = problem.Invalid("group_name_required", "Informe o nome do grupo.")
	ErrGroupNameTooLong   = problem.Invalid("group_name_too_long", "Use no máximo %d caracteres no nome do grupo.", maxGroupName)
	ErrGroupNameTaken     = problem.Conflict("group_name_taken", "Já existe um grupo com esse nome.")
	ErrAlreadyMember      = problem.Conflict("group_already_member", "Essa pessoa já faz parte do grupo.")
	ErrNotMember          = problem.Invalid("group_not_member", "Essa pessoa não faz parte do grupo.")
	ErrInvalidRole        = problem.Invalid("group_role_invalid", "Escolha administrador ou membro.")
	ErrLastAdmin          = problem.Conflict("group_last_admin", "O grupo precisa de pelo menos um administrador.")
	ErrInvalidPIN         = problem.Invalid("pin_invalid", "O PIN tem de 4 a 6 números.")
	ErrNoPIN              = problem.Conflict("pin_not_set", "Seu PIN ainda não foi definido. Peça a um administrador do grupo.")
	ErrWrongPIN           = problem.Invalid("pin_wrong", "PIN incorreto.")
	ErrLocked             = problem.Conflict("pin_locked", "Muitas tentativas erradas. Espere 15 minutos para tentar de novo.")
	ErrLoginTaken         = problem.Conflict("login_taken", "Já existe alguém com esse login.")
	ErrLoginOrPIN         = problem.Invalid("login_or_pin_wrong", "Login ou PIN incorreto.")
	ErrResetOutsideGroups = problem.Forbidden("pin_reset_other_groups", "Essa pessoa também está num grupo que você não administra. O PIN dela só pode ser redefinido por quem administra todos os grupos dela, ou pelo terminal.")
	ErrPersonRequired     = problem.Invalid("person_required", "Informe o nome e o login da pessoa.")
	ErrPersonalGroup      = problem.Conflict("group_personal", "O espaço Pessoal é só seu: não tem outros membros, papéis nem outro nome.")
)

// Group is a group of people with the role of whoever reads it; a personal one is the space of one person (RF-83).
type Group struct {
	ID       int64
	Name     string
	Role     Role
	Personal bool
}

// Member is a person of a group with role, weight and whether a PIN exists.
type Member struct {
	UserID   int64
	PersonID int64
	Login    string
	Name     string
	Role     Role
	WeightBP int64
	HasPIN   bool
}

// CreateGroup creates a group with the user as its first administrator (RF-78, RF-80).
func (s *Service) CreateGroup(ctx context.Context, name string, adminUserID int64) (Group, error) {
	name, err := cleanGroupName(name)
	if err != nil {
		return Group{}, err
	}
	var g store.Group
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		u, err := q.GetUserByID(ctx, adminUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := checkGroupName(ctx, q, name, 0); err != nil {
			return err
		}
		now := s.now().UTC()
		g, err = q.CreateGroup(ctx, store.CreateGroupParams{Name: name, CreatedBy: sql.NullInt64{Int64: adminUserID, Valid: true}, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		if err := q.AddGroupMember(ctx, store.AddGroupMemberParams{GroupID: g.ID, PersonID: u.PersonID, Role: string(RoleAdmin), WeightBp: fullWeightBP, JoinedAt: now}); err != nil {
			return fmt.Errorf("add first member: %w", err)
		}
		return ensurePersonal(ctx, q, u.PersonID, u.ID, now)
	})
	if err != nil {
		return Group{}, err
	}
	return Group{ID: g.ID, Name: g.Name, Role: RoleAdmin}, nil
}

// RenameGroup changes the name of a group (RF-80).
func (s *Service) RenameGroup(ctx context.Context, groupID int64, name string) error {
	name, err := cleanGroupName(name)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := sharedGroup(ctx, q, groupID); err != nil {
			return err
		}
		if err := checkGroupName(ctx, q, name, groupID); err != nil {
			return err
		}
		return q.RenameGroup(ctx, store.RenameGroupParams{Name: name, ID: groupID})
	})
}

// Groups lists every group by name, for the first step of entering (RF-01).
func (s *Service) Groups(ctx context.Context) ([]Group, error) {
	rows, err := store.New(s.conn).ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, Group{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// GetGroup returns one group.
func (s *Service) GetGroup(ctx context.Context, groupID int64) (Group, error) {
	g, err := getGroup(ctx, store.New(s.conn), groupID)
	if err != nil {
		return Group{}, err
	}
	return Group{ID: g.ID, Name: g.Name, Personal: g.Kind == kindPersonal}, nil
}

// GroupsOf lists the groups of a user with the role in each, for the group selector (RF-81).
func (s *Service) GroupsOf(ctx context.Context, userID int64) ([]Group, error) {
	q := store.New(s.conn)
	u, err := q.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGroupsOfPerson(ctx, u.PersonID)
	if err != nil {
		return nil, fmt.Errorf("list groups of person: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, Group{ID: r.ID, Name: r.Name, Role: Role(r.Role), Personal: r.Kind == kindPersonal})
	}
	return out, nil
}

// Members lists the people of a group in the order they joined.
func (s *Service) Members(ctx context.Context, groupID int64) ([]Member, error) {
	rows, err := store.New(s.conn).ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, Member{UserID: r.UserID, PersonID: r.PersonID, Login: r.Login, Name: r.PersonName, Role: Role(r.Role), WeightBP: r.WeightBp, HasPIN: r.HasPin == 1})
	}
	return out, nil
}

// Membership returns the role of the user in the group and whether the user belongs to it.
func (s *Service) Membership(ctx context.Context, groupID, userID int64) (Role, bool, error) {
	return membership(ctx, store.New(s.conn), groupID, userID)
}

// AddMember puts an existing person in the group and makes the weights equal again (RF-79, RF-80).
func (s *Service) AddMember(ctx context.Context, groupID, userID int64, role Role) error {
	if role != RoleAdmin && role != RoleMember {
		return ErrInvalidRole
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := sharedGroup(ctx, q, groupID); err != nil {
			return err
		}
		u, err := q.GetUserByID(ctx, userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, ok, err := membership(ctx, q, groupID, userID); err != nil {
			return err
		} else if ok {
			return ErrAlreadyMember
		}
		now := s.now().UTC()
		if err := q.AddGroupMember(ctx, store.AddGroupMemberParams{GroupID: groupID, PersonID: u.PersonID, Role: string(role), WeightBp: 1, JoinedAt: now}); err != nil {
			return fmt.Errorf("add member: %w", err)
		}
		if err := ensurePersonal(ctx, q, u.PersonID, u.ID, now); err != nil {
			return err
		}
		return equalWeights(ctx, q, groupID)
	})
}

// CreateMember registers a person with login and PIN and puts it in the group, all in one transaction (RF-80, RN-27).
func (s *Service) CreateMember(ctx context.Context, groupID int64, login, name, pin string, role Role) (User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	name = strings.TrimSpace(name)
	switch {
	case login == "" || name == "":
		return User{}, ErrPersonRequired
	case !auth.ValidPIN(pin):
		return User{}, ErrInvalidPIN
	case role != RoleAdmin && role != RoleMember:
		return User{}, ErrInvalidRole
	}
	hash, err := auth.HashPIN(pin)
	if err != nil {
		return User{}, err
	}
	var u store.User
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := sharedGroup(ctx, q, groupID); err != nil {
			return err
		}
		if _, err := q.GetUserByLogin(ctx, login); err == nil {
			return ErrLoginTaken
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := s.now().UTC()
		p, err := q.CreatePerson(ctx, store.CreatePersonParams{Name: name, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("create person: %w", err)
		}
		if u, err = q.CreateUser(ctx, store.CreateUserParams{Login: login, PersonID: p.ID, CreatedAt: now, UpdatedAt: now}); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if err := q.SetUserPIN(ctx, store.SetUserPINParams{PinHash: sql.NullString{String: hash, Valid: true}, UpdatedAt: now, ID: u.ID}); err != nil {
			return err
		}
		if err := q.AddGroupMember(ctx, store.AddGroupMemberParams{GroupID: groupID, PersonID: p.ID, Role: string(role), WeightBp: 1, JoinedAt: now}); err != nil {
			return fmt.Errorf("add member: %w", err)
		}
		if err := ensurePersonal(ctx, q, p.ID, u.ID, now); err != nil {
			return err
		}
		return equalWeights(ctx, q, groupID)
	})
	if err != nil {
		return User{}, err
	}
	return User{ID: u.ID, PersonID: u.PersonID, Login: u.Login, Name: name, Theme: u.Theme, Haptics: u.Haptics}, nil
}

// SetRole changes the role of a member, keeping at least one administrator (RF-80).
func (s *Service) SetRole(ctx context.Context, groupID, userID int64, role Role) error {
	if role != RoleAdmin && role != RoleMember {
		return ErrInvalidRole
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := sharedGroup(ctx, q, groupID); err != nil {
			return err
		}
		members, err := q.ListGroupMembers(ctx, groupID)
		if err != nil {
			return err
		}
		var target *store.ListGroupMembersRow
		admins := 0
		for i, m := range members {
			if m.UserID == userID {
				target = &members[i]
			}
			if Role(m.Role) == RoleAdmin {
				admins++
			}
		}
		if target == nil {
			return ErrNotMember
		}
		if Role(target.Role) == RoleAdmin && role == RoleMember && admins == 1 {
			return ErrLastAdmin
		}
		return q.SetGroupMemberRole(ctx, store.SetGroupMemberRoleParams{Role: string(role), GroupID: groupID, PersonID: target.PersonID})
	})
}

// SetPIN stores a new PIN for the user and lifts any lock (RF-80, RN-27).
func (s *Service) SetPIN(ctx context.Context, userID int64, pin string) error {
	if !auth.ValidPIN(pin) {
		return ErrInvalidPIN
	}
	hash, err := auth.HashPIN(pin)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetUserByID(ctx, userID); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return q.SetUserPIN(ctx, store.SetUserPINParams{PinHash: sql.NullString{String: hash, Valid: true}, UpdatedAt: s.now().UTC(), ID: userID})
	})
}

// Authenticate checks the PIN of a member of the group: five wrong ones in a row lock the person out for
// 15 minutes, and a right one clears the count (RF-01, RN-27).
func (s *Service) Authenticate(ctx context.Context, groupID, userID int64, pin string) (User, error) {
	q := store.New(s.conn)
	if _, err := q.GetUserByID(ctx, userID); errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	} else if err != nil {
		return User{}, err
	}
	if _, ok, err := membership(ctx, q, groupID, userID); err != nil {
		return User{}, err
	} else if !ok {
		return User{}, ErrNotMember
	}
	// A personal space is never a way in: one enters through a shared group and switches to it (RF-01, RF-83).
	if g, err := getGroup(ctx, q, groupID); err != nil {
		return User{}, err
	} else if g.Kind == kindPersonal {
		return User{}, ErrNotMember
	}
	if err := s.verifyPIN(ctx, userID, pin); err != nil {
		return User{}, err
	}
	return s.Get(ctx, userID)
}

// JoinWithPIN brings a person already registered into the group when that person types the own PIN, as consent;
// an unknown login, a missing PIN and a wrong PIN give the same answer, so nobody learns which logins exist (RF-80, RN-27).
func (s *Service) JoinWithPIN(ctx context.Context, groupID int64, login, pin string, role Role) (User, error) {
	if role != RoleAdmin && role != RoleMember {
		return User{}, ErrInvalidRole
	}
	if g, err := s.GetGroup(ctx, groupID); err != nil {
		return User{}, err
	} else if g.Personal {
		return User{}, ErrPersonalGroup
	}
	u, err := s.GetByLogin(ctx, strings.ToLower(strings.TrimSpace(login)))
	if errors.Is(err, ErrNotFound) {
		// The same slow check as a real PIN, so the answer time does not reveal the missing login.
		auth.CheckPIN(dummyPINHash(), pin)
		return User{}, ErrLoginOrPIN
	}
	if err != nil {
		return User{}, err
	}
	if err := s.verifyPIN(ctx, u.ID, pin); errors.Is(err, ErrWrongPIN) || errors.Is(err, ErrNoPIN) {
		return User{}, ErrLoginOrPIN
	} else if err != nil {
		return User{}, err
	}
	if err := s.AddMember(ctx, groupID, u.ID, role); err != nil {
		return User{}, err
	}
	return u, nil
}

// CanResetPIN allows an administrator to reset the PIN of a person only when the administrator runs every group
// of that person, so no group reaches the login used in another (RF-80).
func (s *Service) CanResetPIN(ctx context.Context, callerID, targetID int64) error {
	groups, err := s.GroupsOf(ctx, targetID)
	if err != nil {
		return err
	}
	shared := 0
	for _, g := range groups {
		// The personal space has only its owner, so it never counts against the administrator.
		if g.Personal {
			continue
		}
		shared++
		role, ok, err := s.Membership(ctx, g.ID, callerID)
		if err != nil {
			return err
		}
		if !ok || role != RoleAdmin {
			return ErrResetOutsideGroups
		}
	}
	if shared == 0 {
		return ErrResetOutsideGroups
	}
	return nil
}

// verifyPIN checks a PIN with the lock of RN-27. The slow hash runs outside the write transaction, which then
// reads the count again and records the outcome, so a lock set meanwhile by another try still wins.
func (s *Service) verifyPIN(ctx context.Context, userID int64, pin string) error {
	row, err := store.New(s.conn).GetUserPIN(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.PinLockedUntil.Valid && s.now().UTC().Before(row.PinLockedUntil.Time) {
		return ErrLocked
	}
	if !row.PinHash.Valid {
		return ErrNoPIN
	}
	right := auth.CheckPIN(row.PinHash.String, pin)
	var outcome error
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := q.GetUserPIN(ctx, userID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		switch {
		case cur.PinLockedUntil.Valid && now.Before(cur.PinLockedUntil.Time):
			outcome = ErrLocked
			return nil
		case cur.PinHash != row.PinHash:
			// The PIN changed while this one was checked: the check was against the old one.
			outcome = ErrWrongPIN
			return nil
		case right:
			return q.ClearPINFailures(ctx, userID)
		}
		failures := cur.PinFailures + 1
		locked := sql.NullTime{}
		outcome = ErrWrongPIN
		if failures >= maxPINFailures {
			failures, locked, outcome = 0, sql.NullTime{Time: now.Add(pinLockout), Valid: true}, ErrLocked
		}
		return q.RecordPINFailure(ctx, store.RecordPINFailureParams{PinFailures: failures, PinLockedUntil: locked, ID: userID})
	})
	if err != nil {
		return err
	}
	return outcome
}

// dummyHash is a hash of no real PIN, checked when a login does not exist so the answer takes as long.
var dummyHash struct {
	once sync.Once
	hash string
}

// dummyPINHash returns the hash used for missing logins, made on first use with the current rounds.
func dummyPINHash() string {
	dummyHash.once.Do(func() {
		dummyHash.hash, _ = auth.HashPIN("000000")
	})
	return dummyHash.hash
}

// cleanGroupName trims and checks a group name.
func cleanGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrGroupNameRequired
	case utf8.RuneCountInString(name) > maxGroupName:
		return "", ErrGroupNameTooLong
	}
	return name, nil
}

// checkGroupName refuses a name another group already has, ignoring case.
func checkGroupName(ctx context.Context, q *store.Queries, name string, self int64) error {
	groups, err := q.ListGroups(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if g.ID != self && strings.EqualFold(g.Name, name) {
			return ErrGroupNameTaken
		}
	}
	return nil
}

// getGroup reads a group or returns ErrGroupNotFound.
func getGroup(ctx context.Context, q *store.Queries, id int64) (store.Group, error) {
	g, err := q.GetGroup(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Group{}, ErrGroupNotFound
	}
	return g, err
}

// sharedGroup reads a group that takes members, refusing a personal space (RF-83).
func sharedGroup(ctx context.Context, q *store.Queries, id int64) (store.Group, error) {
	g, err := getGroup(ctx, q, id)
	if err != nil {
		return store.Group{}, err
	}
	if g.Kind == kindPersonal {
		return store.Group{}, ErrPersonalGroup
	}
	return g, nil
}

// ensurePersonal gives the person her personal space, with her as its only member, if she has none yet (RF-83).
func ensurePersonal(ctx context.Context, q *store.Queries, personID, userID int64, now time.Time) error {
	if err := q.EnsurePersonalGroup(ctx, store.EnsurePersonalGroupParams{PersonID: sql.NullInt64{Int64: personID, Valid: true}, CreatedBy: sql.NullInt64{Int64: userID, Valid: true}, CreatedAt: now}); err != nil {
		return fmt.Errorf("personal group: %w", err)
	}
	if err := q.EnsurePersonalMember(ctx, store.EnsurePersonalMemberParams{JoinedAt: now, PersonID: sql.NullInt64{Int64: personID, Valid: true}}); err != nil {
		return fmt.Errorf("personal member: %w", err)
	}
	return nil
}

// membership returns the role of the user in the group and whether the user belongs to it.
func membership(ctx context.Context, q *store.Queries, groupID, userID int64) (Role, bool, error) {
	u, err := q.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	m, err := q.GetGroupMember(ctx, store.GetGroupMemberParams{GroupID: groupID, PersonID: u.PersonID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return Role(m.Role), true, nil
}

// equalWeights gives every member the same weight, the leftover points to the first one; custom weights
// come with the settlement between members (Fase 10, RN-28).
func equalWeights(ctx context.Context, q *store.Queries, groupID int64) error {
	members, err := q.ListGroupMembers(ctx, groupID)
	if err != nil {
		return err
	}
	n := int64(len(members))
	for i, m := range members {
		w := fullWeightBP / n
		if i == 0 {
			w += fullWeightBP % n
		}
		if err := q.SetGroupMemberWeight(ctx, store.SetGroupMemberWeightParams{WeightBp: w, GroupID: groupID, PersonID: m.PersonID}); err != nil {
			return err
		}
	}
	return nil
}
