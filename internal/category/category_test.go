package category

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/db"
)

// fixture holds a migrated database with the seed, one user and the service.
type fixture struct {
	s      *Service
	conn   *sql.DB
	userID int64
}

// newFixture opens a fresh migrated database for one test.
func newFixture(t *testing.T) fixture {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	u, err := account.NewService(conn).Create(context.Background(), "ana", "Ana Ficticia")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{s: NewService(conn), conn: conn, userID: u.ID}
}

// find returns the category with the given name under the given parent name ("" for top level).
func (f fixture) find(t *testing.T, parent, name string) Category {
	t.Helper()
	tree, err := f.s.List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range tree {
		if parent == "" && c.Name == name {
			return c
		}
		if c.Name == parent {
			for _, sub := range c.Children {
				if sub.Name == name {
					return sub
				}
			}
		}
	}
	t.Fatalf("category %q > %q not found", parent, name)
	return Category{}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// TestSeed checks the initial list of section 6 and its icons (section 5.6).
func TestSeed(t *testing.T) {
	f := newFixture(t)
	tree, err := f.s.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 13 {
		t.Fatalf("top level = %d, want 13", len(tree))
	}
	tests := []struct {
		name, icon string
		subs       int
		firstSub   string
	}{
		{name: "Moradia", icon: "house", subs: 8, firstSub: "Financiamento da casa"},
		{name: "Alimentação", icon: "utensils", subs: 3, firstSub: "Compras para casa"},
		{name: "Carro", icon: "car", subs: 7, firstSub: "Financiamento"},
		{name: "Outros", icon: "ellipsis", subs: 1, firstSub: "Sem categoria definida"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := f.find(t, "", tt.name)
			if c.Icon != tt.icon || len(c.Children) != tt.subs || c.Children[0].Name != tt.firstSub {
				t.Fatalf("category = %+v", c)
			}
			if c.ParentID != nil || *c.Children[0].ParentID != c.ID {
				t.Fatal("wrong parent links")
			}
		})
	}
	if tree[0].Name != "Moradia" || tree[12].Name != "Outros" {
		t.Fatalf("order = %s ... %s", tree[0].Name, tree[12].Name)
	}
}

// TestCreate covers creating categories and subcategories (RF-50).
func TestCreate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	moradia := f.find(t, "", "Moradia")
	energia := f.find(t, "Moradia", "Energia")
	inactive, err := f.s.Create(ctx, f.userID, NewCategory{Name: "Antiga", Icon: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Update(ctx, inactive.ID, Changes{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		in      NewCategory
		wantErr error
	}{
		{name: "top level with icon", in: NewCategory{Name: "  Doações ", Icon: "gift"}},
		{name: "top level without icon", in: NewCategory{Name: "Igreja"}, wantErr: ErrIconRequired},
		{name: "invalid icon", in: NewCategory{Name: "Igreja", Icon: "Casa Grande"}, wantErr: ErrInvalidIcon},
		{name: "subcategory without icon", in: NewCategory{Name: "Gás", ParentID: &moradia.ID}},
		{name: "same name under another parent", in: NewCategory{Name: "Financiamento", ParentID: &moradia.ID}},
		{name: "duplicate ignoring case, with accents", in: NewCategory{Name: "ÁGUA", ParentID: &moradia.ID}, wantErr: ErrDuplicateName},
		{name: "duplicate top level", in: NewCategory{Name: "lazer", Icon: "ticket"}, wantErr: ErrDuplicateName},
		{name: "empty name", in: NewCategory{Name: "   ", Icon: "gift"}, wantErr: ErrNameRequired},
		{name: "name too long", in: NewCategory{Name: strings.Repeat("a", 61), Icon: "gift"}, wantErr: ErrNameTooLong},
		{name: "parent is a subcategory", in: NewCategory{Name: "Conta de luz", ParentID: &energia.ID}, wantErr: ErrParentNotTopLevel},
		{name: "parent missing", in: NewCategory{Name: "Gás", ParentID: ptr(int64(9999))}, wantErr: ErrParentNotFound},
		{name: "parent inactive", in: NewCategory{Name: "Coisa", ParentID: &inactive.ID}, wantErr: ErrParentInactive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := f.s.Create(ctx, f.userID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if c.Name != strings.TrimSpace(tt.in.Name) || !c.Active {
				t.Fatalf("created = %+v", c)
			}
			var createdBy int64
			if err := f.conn.QueryRow("SELECT created_by FROM categories WHERE id = ?", c.ID).Scan(&createdBy); err != nil || createdBy != f.userID {
				t.Fatalf("created_by = %d, %v (RF-03)", createdBy, err)
			}
		})
	}

	gas := f.find(t, "Moradia", "Gás")
	if last := f.find(t, "", "Moradia").Children; last[len(last)-2].ID != gas.ID {
		t.Fatal("new subcategory should go to the end of its parent")
	}
}

// TestUpdate covers renaming, icons and deactivation (RF-50, RN-18).
func TestUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lazer := f.find(t, "", "Lazer")
	viagens := f.find(t, "Lazer", "Viagens")

	tests := []struct {
		name    string
		id      int64
		changes Changes
		wantErr error
		check   func(t *testing.T, c Category)
	}{
		{
			name: "rename subcategory", id: viagens.ID, changes: Changes{Name: ptr(" Viagens e férias ")},
			check: func(t *testing.T, c Category) {
				if c.Name != "Viagens e férias" {
					t.Fatalf("name = %q", c.Name)
				}
			},
		},
		{name: "rename to sibling name", id: viagens.ID, changes: Changes{Name: ptr("hobbies")}, wantErr: ErrDuplicateName},
		{name: "rename keeping own name in another case", id: lazer.ID, changes: Changes{Name: ptr("LAZER")}},
		{name: "empty name", id: lazer.ID, changes: Changes{Name: ptr("")}, wantErr: ErrNameRequired},
		{name: "change icon", id: lazer.ID, changes: Changes{Icon: ptr("party-popper")}},
		{name: "top level cannot lose icon", id: lazer.ID, changes: Changes{Icon: ptr("")}, wantErr: ErrIconRequired},
		{name: "subcategory may have no icon", id: viagens.ID, changes: Changes{Icon: ptr("")}},
		{name: "missing category", id: 9999, changes: Changes{Name: ptr("x")}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := f.s.Update(ctx, tt.id, tt.changes)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

// TestDeactivate checks that inactive categories leave the options but stay in history (RN-18).
func TestDeactivate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pets := f.find(t, "", "Pets")
	racao := f.find(t, "Pets", "Ração")
	streaming := f.find(t, "Assinaturas", "Streaming")

	if _, err := f.s.Update(ctx, streaming.ID, Changes{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Update(ctx, pets.ID, Changes{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	active, _ := f.s.List(ctx, false)
	all, _ := f.s.List(ctx, true)
	tests := []struct {
		name       string
		tree       []Category
		id         int64
		wantListed bool
	}{
		{name: "inactive subcategory hidden from options", tree: active, id: streaming.ID, wantListed: false},
		{name: "inactive parent hidden from options", tree: active, id: pets.ID, wantListed: false},
		{name: "child of inactive parent hidden", tree: active, id: racao.ID, wantListed: false},
		{name: "inactive subcategory kept in full list", tree: all, id: streaming.ID, wantListed: true},
		{name: "inactive parent kept in full list", tree: all, id: pets.ID, wantListed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listed(tt.tree, tt.id); got != tt.wantListed {
				t.Fatalf("listed = %v, want %v", got, tt.wantListed)
			}
		})
	}

	if c, err := f.s.Update(ctx, pets.ID, Changes{Active: ptr(true)}); err != nil || !c.Active {
		t.Fatalf("reactivate = %+v, %v", c, err)
	}
	if active, _ := f.s.List(ctx, false); !listed(active, racao.ID) {
		t.Fatal("children should come back with the parent")
	}
}

// listed reports whether id appears anywhere in the tree.
func listed(tree []Category, id int64) bool {
	for _, c := range tree {
		if c.ID == id {
			return true
		}
		for _, sub := range c.Children {
			if sub.ID == id {
				return true
			}
		}
	}
	return false
}

// TestMove covers moving subcategories between categories (RF-50).
func TestMove(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	moradia := f.find(t, "", "Moradia")
	saude := f.find(t, "", "Saúde")
	academia := f.find(t, "Saúde", "Academia")
	financiamentoCarro := f.find(t, "Carro", "Financiamento")
	presentes := f.find(t, "Compras", "Presentes")
	empty, err := f.s.Create(ctx, f.userID, NewCategory{Name: "Vazia", Icon: "box"})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := f.s.Create(ctx, f.userID, NewCategory{Name: "Fechada", Icon: "lock"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Update(ctx, closed.ID, Changes{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	lazer := f.find(t, "", "Lazer")
	groceries := f.find(t, "Alimentação", "Compras para casa")
	delivery := f.find(t, "Alimentação", "Delivery")
	if _, err := f.conn.Exec(`INSERT INTO alias_rules (original, friendly_name, category_id, created_by, created_at) VALUES ('ifood', 'iFood', ?, ?, '2026-11-11')`, delivery.ID, f.userID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		id      int64
		in      Move
		wantErr error
		check   func(t *testing.T, c Category)
	}{
		{
			name: "subcategory to another parent", id: academia.ID, in: Move{ParentID: &lazer.ID},
			check: func(t *testing.T, c Category) {
				kids := f.find(t, "", "Lazer").Children
				if *c.ParentID != lazer.ID || kids[len(kids)-1].ID != academia.ID {
					t.Fatalf("moved = %+v", c)
				}
			},
		},
		{name: "to itself", id: moradia.ID, in: Move{ParentID: &moradia.ID}, wantErr: ErrParentNotTopLevel},
		{name: "under a subcategory", id: presentes.ID, in: Move{ParentID: &financiamentoCarro.ID}, wantErr: ErrParentNotTopLevel},
		{name: "top level with children under another", id: saude.ID, in: Move{ParentID: &moradia.ID}, wantErr: ErrHasChildren},
		{name: "similar name at destination", id: financiamentoCarro.ID, in: Move{ParentID: &moradia.ID}},
		{name: "inactive destination", id: presentes.ID, in: Move{ParentID: &closed.ID}, wantErr: ErrParentInactive},
		{name: "missing destination", id: presentes.ID, in: Move{ParentID: ptr(int64(9999))}, wantErr: ErrParentNotFound},
		{name: "missing category", id: 9999, in: Move{ParentID: &moradia.ID}, wantErr: ErrNotFound},
		{name: "empty top level becomes subcategory", id: empty.ID, in: Move{ParentID: &moradia.ID}},
		{name: "promote without icon", id: presentes.ID, in: Move{}, wantErr: ErrIconRequired},
		{
			name: "promote with icon", id: presentes.ID, in: Move{Icon: "gift"},
			check: func(t *testing.T, c Category) {
				if c.ParentID != nil || c.Icon != "gift" || f.find(t, "", "Presentes").ID != presentes.ID {
					t.Fatalf("promoted = %+v", c)
				}
			},
		},
		{name: "promote a system subcategory (RN-15, RN-26)", id: groceries.ID, in: Move{Icon: "cart"}, wantErr: ErrCannotPromote},
		{name: "promote a subcategory in use", id: delivery.ID, in: Move{Icon: "bike"}, wantErr: ErrCannotPromote},
		{name: "system subcategory to another parent", id: groceries.ID, in: Move{ParentID: &lazer.ID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := f.s.Move(ctx, tt.id, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, c)
			}
		})
	}

	// "Financiamento" moved from Carro to Moradia, where "Financiamento da casa" exists:
	// the names differ, so it is allowed. Moving a second one with the same name is not.
	other, err := f.s.Create(ctx, f.userID, NewCategory{Name: "Financiamento", ParentID: &saude.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Move(ctx, other.ID, Move{ParentID: &moradia.ID}); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("duplicate move: err = %v, want ErrDuplicateName", err)
	}
}

// TestDelete checks that only unused categories can be deleted (RN-18).
func TestDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// Stand-in for the entry tables of later steps, which reference categories the same way.
	if _, err := f.conn.Exec(`CREATE TABLE fake_entries (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL REFERENCES categories (id))`); err != nil {
		t.Fatal(err)
	}
	used := f.find(t, "Alimentação", "Delivery")
	if _, err := f.conn.Exec(`INSERT INTO fake_entries (category_id) VALUES (?)`, used.ID); err != nil {
		t.Fatal(err)
	}
	unused := f.find(t, "Educação", "Cursos")
	educacao := f.find(t, "", "Educação")
	groceries := f.find(t, "Alimentação", "Compras para casa")

	tests := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{name: "unused subcategory", id: unused.ID},
		{name: "subcategory with entries", id: used.ID, wantErr: ErrInUse},
		{name: "category with subcategories", id: educacao.ID, wantErr: ErrHasChildren},
		{name: "system subcategory used by Alelo purchases (RN-15)", id: groceries.ID, wantErr: ErrSystemCategory},
		{name: "missing category", id: 9999, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := f.s.Delete(ctx, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			all, _ := f.s.List(ctx, true)
			if stillThere := listed(all, tt.id); stillThere != (tt.wantErr != nil && tt.id != 9999) {
				t.Fatalf("still listed = %v", stillThere)
			}
		})
	}
}
