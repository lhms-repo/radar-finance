package account

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/db"
)

// newGroupService opens a fresh database with a clock the test controls, and fast PIN hashing.
func newGroupService(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	old := auth.PINIterations
	auth.PINIterations = 1_000
	t.Cleanup(func() { auth.PINIterations = old })
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	s := NewService(conn)
	s.now = func() time.Time { return now }
	return s, &now
}

// TestGroups creates groups, adds members with equal weights and keeps at least one administrator (RF-78 to RF-80).
func TestGroups(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	bruno, _ := s.Create(ctx, "bruno", "Bruno Ficticio")
	carla, _ := s.Create(ctx, "carla", "Carla Ficticia")

	home, err := s.CreateGroup(ctx, "Casa", ana.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGroup(ctx, "casa", bruno.ID); !errors.Is(err, ErrGroupNameTaken) {
		t.Fatalf("same name, other case: %v, want ErrGroupNameTaken", err)
	}
	if _, err := s.CreateGroup(ctx, "  ", ana.ID); !errors.Is(err, ErrGroupNameRequired) {
		t.Fatalf("empty name: %v", err)
	}
	if err := s.AddMember(ctx, home.ID, bruno.ID, RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(ctx, home.ID, bruno.ID, RoleMember); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("twice: %v", err)
	}
	if err := s.AddMember(ctx, home.ID, carla.ID, RoleMember); err != nil {
		t.Fatal(err)
	}
	members, err := s.Members(ctx, home.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 10000 basis points in three: the leftover point goes to the first member.
	want := []struct {
		login  string
		role   Role
		weight int64
	}{{"ana", RoleAdmin, 3_334}, {"bruno", RoleMember, 3_333}, {"carla", RoleMember, 3_333}}
	if len(members) != len(want) {
		t.Fatalf("members = %+v", members)
	}
	for i, w := range want {
		if m := members[i]; m.Login != w.login || m.Role != w.role || m.WeightBP != w.weight || m.HasPIN {
			t.Errorf("member %d = %+v, want %+v", i, m, w)
		}
	}

	if err := s.SetRole(ctx, home.ID, ana.ID, RoleMember); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("removing the only admin: %v", err)
	}
	if err := s.SetRole(ctx, home.ID, bruno.ID, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRole(ctx, home.ID, ana.ID, RoleMember); err != nil {
		t.Fatalf("an admin is left: %v", err)
	}
	if err := s.SetRole(ctx, home.ID, carla.ID, "owner"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("unknown role: %v", err)
	}

	beach, _ := s.CreateGroup(ctx, "Casa de praia", carla.ID)
	groups, err := s.GroupsOf(ctx, carla.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The personal space comes first (RF-83), then the shared groups by name.
	if len(groups) != 3 || !groups[0].Personal || groups[1].ID != home.ID || groups[1].Role != RoleMember || groups[2].ID != beach.ID || groups[2].Role != RoleAdmin {
		t.Fatalf("groups of Carla = %+v", groups)
	}
	if err := s.RenameGroup(ctx, beach.ID, "Casa"); !errors.Is(err, ErrGroupNameTaken) {
		t.Fatalf("rename onto another name: %v", err)
	}
	if err := s.RenameGroup(ctx, beach.ID, "Praia"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Membership(ctx, beach.ID, ana.ID); ok {
		t.Fatal("Ana is not in the beach group")
	}
}

// TestCreateMember registers a person with login and PIN straight into a group, all or nothing (RF-80).
func TestCreateMember(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	home, _ := s.CreateGroup(ctx, "Casa", ana.ID)
	tests := []struct {
		name    string
		login   string
		person  string
		pin     string
		wantErr error
	}{
		{name: "new person", login: "Carla", person: "Carla Ficticia", pin: "9264"},
		{name: "login taken", login: "ana", person: "Outra Ana", pin: "1234", wantErr: ErrLoginTaken},
		{name: "short PIN", login: "davi", person: "Davi Ficticio", pin: "12", wantErr: ErrInvalidPIN},
		{name: "no name", login: "eva", person: " ", pin: "1234", wantErr: ErrPersonRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := s.CreateMember(ctx, home.ID, tt.login, tt.person, tt.pin, RoleMember)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if u.Login != "carla" {
				t.Fatalf("login = %q, want lowercase", u.Login)
			}
			if _, err := s.Authenticate(ctx, home.ID, u.ID, tt.pin); err != nil {
				t.Fatalf("the new member cannot enter: %v", err)
			}
		})
	}
	// A refused registration leaves nobody behind.
	users, _ := s.List(ctx)
	if len(users) != 2 {
		t.Fatalf("users = %+v, want ana and carla only", users)
	}
}

// TestJoinWithPIN brings a person already registered into another group only with that person's PIN, and never
// tells whether a login exists (RF-80, RN-27).
func TestJoinWithPIN(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	carla, _ := s.Create(ctx, "carla", "Carla Ficticia")
	_ = s.SetPIN(ctx, ana.ID, "4821")
	home, _ := s.CreateGroup(ctx, "Casa", ana.ID)
	beach, _ := s.CreateGroup(ctx, "Praia", carla.ID)

	tests := []struct {
		name    string
		login   string
		pin     string
		wantErr error
	}{
		{name: "unknown login", login: "zeca", pin: "4821", wantErr: ErrLoginOrPIN},
		{name: "wrong PIN", login: "ana", pin: "0000", wantErr: ErrLoginOrPIN},
		{name: "person without a PIN", login: "carla", pin: "1234", wantErr: ErrLoginOrPIN},
		{name: "right PIN", login: "ANA", pin: "4821"},
		{name: "already in the group", login: "ana", pin: "4821", wantErr: ErrAlreadyMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.JoinWithPIN(ctx, beach.ID, tt.login, tt.pin, RoleMember); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if _, ok, _ := s.Membership(ctx, beach.ID, ana.ID); !ok {
		t.Fatal("Ana did not join the beach group")
	}
	// Wrong PINs through this door count for the lock like at the entry.
	for range 4 {
		_, _ = s.JoinWithPIN(ctx, home.ID, "carla", "0000", RoleMember)
	}
	_ = s.SetPIN(ctx, carla.ID, "9264")
	for range 5 {
		_, _ = s.JoinWithPIN(ctx, home.ID, "carla", "0000", RoleMember)
	}
	if _, err := s.JoinWithPIN(ctx, home.ID, "carla", "9264", RoleMember); !errors.Is(err, ErrLocked) {
		t.Fatalf("after five wrong PINs: %v, want ErrLocked", err)
	}
}

// TestCanResetPIN lets an administrator reset only the PIN of someone whose every group the administrator runs (RF-80).
func TestCanResetPIN(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	bruno, _ := s.Create(ctx, "bruno", "Bruno Ficticio")
	carla, _ := s.Create(ctx, "carla", "Carla Ficticia")
	home, _ := s.CreateGroup(ctx, "Casa", ana.ID)
	_ = s.AddMember(ctx, home.ID, bruno.ID, RoleMember)
	beach, _ := s.CreateGroup(ctx, "Praia", carla.ID)
	_ = s.AddMember(ctx, beach.ID, bruno.ID, RoleMember)

	tests := []struct {
		name    string
		caller  int64
		target  int64
		wantErr error
	}{
		{name: "Bruno is also in a group Ana does not run", caller: ana.ID, target: bruno.ID, wantErr: ErrResetOutsideGroups},
		{name: "a member is no administrator", caller: bruno.ID, target: ana.ID, wantErr: ErrResetOutsideGroups},
		{name: "own groups only", caller: carla.ID, target: carla.ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.CanResetPIN(ctx, tt.caller, tt.target); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	// Once Ana also runs the beach group, she may reset the PIN of Bruno.
	_ = s.AddMember(ctx, beach.ID, ana.ID, RoleAdmin)
	if err := s.CanResetPIN(ctx, ana.ID, bruno.ID); err != nil {
		t.Fatalf("admin of every group of Bruno: %v", err)
	}
}

// TestAuthenticate checks the PIN of a member and locks the person out after five wrong ones (RF-01, RN-27).
func TestAuthenticate(t *testing.T) {
	s, now := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	bruno, _ := s.Create(ctx, "bruno", "Bruno Ficticio")
	home, _ := s.CreateGroup(ctx, "Casa", ana.ID)
	other, _ := s.CreateGroup(ctx, "Outro", bruno.ID)

	if err := s.SetPIN(ctx, ana.ID, "12a4"); !errors.Is(err, ErrInvalidPIN) {
		t.Fatalf("letters in the PIN: %v", err)
	}
	if _, err := s.Authenticate(ctx, home.ID, ana.ID, "1234"); !errors.Is(err, ErrNoPIN) {
		t.Fatalf("before a PIN exists: %v", err)
	}
	if err := s.SetPIN(ctx, ana.ID, "4821"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		group   int64
		user    int64
		pin     string
		wantErr error
	}{
		{name: "right PIN", group: home.ID, user: ana.ID, pin: "4821"},
		{name: "not a member of the group", group: other.ID, user: ana.ID, pin: "4821", wantErr: ErrNotMember},
		{name: "unknown person", group: home.ID, user: 999, pin: "4821", wantErr: ErrNotFound},
		{name: "first wrong PIN", group: home.ID, user: ana.ID, pin: "0000", wantErr: ErrWrongPIN},
		{name: "right PIN clears the count", group: home.ID, user: ana.ID, pin: "4821"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := s.Authenticate(ctx, tt.group, tt.user, tt.pin)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && u.ID != tt.user {
				t.Fatalf("user = %+v", u)
			}
		})
	}

	// Five wrong PINs in a row lock Ana out; even the right one is refused until the lock ends.
	for i := 1; i <= 5; i++ {
		_, err := s.Authenticate(ctx, home.ID, ana.ID, "0000")
		want := ErrWrongPIN
		if i == 5 {
			want = ErrLocked
		}
		if !errors.Is(err, want) {
			t.Fatalf("wrong PIN %d: %v, want %v", i, err, want)
		}
	}
	if _, err := s.Authenticate(ctx, home.ID, ana.ID, "4821"); !errors.Is(err, ErrLocked) {
		t.Fatalf("right PIN while locked: %v", err)
	}
	*now = now.Add(14 * time.Minute)
	if _, err := s.Authenticate(ctx, home.ID, ana.ID, "4821"); !errors.Is(err, ErrLocked) {
		t.Fatalf("after 14 minutes: %v", err)
	}
	*now = now.Add(2 * time.Minute)
	if _, err := s.Authenticate(ctx, home.ID, ana.ID, "4821"); err != nil {
		t.Fatalf("after the lock: %v", err)
	}
	// A new PIN set by an administrator also lifts a lock.
	for range 5 {
		_, _ = s.Authenticate(ctx, home.ID, ana.ID, "0000")
	}
	if err := s.SetPIN(ctx, ana.ID, "5555"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, home.ID, ana.ID, "5555"); err != nil {
		t.Fatalf("after a new PIN: %v", err)
	}
}

// TestPersonalSpace gives each person a personal space when she joins her first group, keeps it out of the
// entrance and closed to other members, and leaves PIN resets to the administrator of the shared groups (RF-83).
func TestPersonalSpace(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	bruno, _ := s.Create(ctx, "bruno", "Bruno Ficticio")
	if groups, _ := s.GroupsOf(ctx, ana.ID); len(groups) != 0 {
		t.Fatalf("groups before any group = %+v, want none", groups)
	}
	home, err := s.CreateGroup(ctx, "Casa", ana.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(ctx, home.ID, bruno.ID, RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPIN(ctx, bruno.ID, "7351"); err != nil {
		t.Fatal(err)
	}

	personal := map[int64]Group{}
	for _, u := range []User{ana, bruno} {
		groups, err := s.GroupsOf(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 2 || !groups[0].Personal || groups[0].Name != "Pessoal" || groups[0].Role != RoleAdmin || groups[1].ID != home.ID || groups[1].Personal {
			t.Fatalf("groups of %s = %+v, want Pessoal first and then Casa", u.Login, groups)
		}
		personal[u.ID] = groups[0]
	}
	if personal[ana.ID].ID == personal[bruno.ID].ID {
		t.Fatal("ana and bruno share a personal space")
	}
	if members, _ := s.Members(ctx, personal[ana.ID].ID); len(members) != 1 || members[0].UserID != ana.ID || members[0].WeightBP != fullWeightBP {
		t.Fatalf("members of the personal space = %+v, want only ana", members)
	}
	// Joining another group keeps the same personal space.
	if _, err := s.CreateGroup(ctx, "Praia", ana.ID); err != nil {
		t.Fatal(err)
	}
	if groups, _ := s.GroupsOf(ctx, ana.ID); len(groups) != 3 || groups[0].ID != personal[ana.ID].ID {
		t.Fatalf("groups after a second group = %+v", groups)
	}
	// The entrance lists only shared groups.
	entrance, _ := s.Groups(ctx)
	for _, g := range entrance {
		if g.Personal || g.Name == "Pessoal" {
			t.Fatalf("entrance lists a personal space: %+v", entrance)
		}
	}
	if len(entrance) != 2 {
		t.Fatalf("entrance = %+v, want Casa and Praia", entrance)
	}

	mine := personal[ana.ID].ID
	refused := []struct {
		name string
		err  error
	}{
		{"rename", s.RenameGroup(ctx, mine, "Meu")},
		{"add member", s.AddMember(ctx, mine, bruno.ID, RoleMember)},
		{"set role", s.SetRole(ctx, mine, ana.ID, RoleMember)},
	}
	if _, err := s.CreateMember(ctx, mine, "carla", "Carla Ficticia", "9264", RoleMember); true {
		refused = append(refused, struct {
			name string
			err  error
		}{"create member", err})
	}
	if _, err := s.JoinWithPIN(ctx, mine, "bruno", "7351", RoleMember); true {
		refused = append(refused, struct {
			name string
			err  error
		}{"join with PIN", err})
	}
	for _, r := range refused {
		if !errors.Is(r.err, ErrPersonalGroup) {
			t.Errorf("%s in a personal space: %v, want ErrPersonalGroup", r.name, r.err)
		}
	}
	// Ana runs every shared group of Bruno, so his personal space does not stop her from resetting his PIN.
	if err := s.CanResetPIN(ctx, ana.ID, bruno.ID); err != nil {
		t.Fatalf("reset by the administrator of all shared groups: %v", err)
	}
	if err := s.CanResetPIN(ctx, bruno.ID, ana.ID); !errors.Is(err, ErrResetOutsideGroups) {
		t.Fatalf("reset by a member: %v, want ErrResetOutsideGroups", err)
	}
}

// TestPersonalNameReserved keeps the name of the personal space out of shared groups, so no picker shows two (RF-83).
func TestPersonalNameReserved(t *testing.T) {
	s, _ := newGroupService(t)
	ctx := context.Background()
	ana, _ := s.Create(ctx, "ana", "Ana Ficticia")
	if _, err := s.CreateGroup(ctx, " pessoal ", ana.ID); !errors.Is(err, ErrGroupNameReserved) {
		t.Fatalf("create Pessoal: %v, want ErrGroupNameReserved", err)
	}
	home, err := s.CreateGroup(ctx, "Casa", ana.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenameGroup(ctx, home.ID, "PESSOAL"); !errors.Is(err, ErrGroupNameReserved) {
		t.Fatalf("rename to Pessoal: %v, want ErrGroupNameReserved", err)
	}
}
