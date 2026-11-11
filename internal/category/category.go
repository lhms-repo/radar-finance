// Package category manages categories and subcategories (RF-50, RN-18).
//
// The tree has two levels. Top level categories carry an icon for the quick
// entry grid (section 5.6); subcategories classify the entries.
package category

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// maxNameLength is the longest category name accepted, in characters.
const maxNameLength = 60

// iconPattern accepts Lucide icon names such as "heart-pulse".
var iconPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Business errors returned by the service.
var (
	ErrNotFound          = problem.NotFound("category_not_found", "Categoria não encontrada.")
	ErrNameRequired      = problem.Invalid("category_name_required", "Informe o nome da categoria.")
	ErrNameTooLong       = problem.Invalid("category_name_too_long", "Use no máximo %d caracteres no nome.", maxNameLength)
	ErrIconRequired      = problem.Invalid("category_icon_required", "Escolha um ícone para a categoria principal.")
	ErrInvalidIcon       = problem.Invalid("category_icon_invalid", "Ícone inválido.")
	ErrParentNotFound    = problem.Invalid("category_parent_not_found", "A categoria principal escolhida não existe.")
	ErrParentNotTopLevel = problem.Invalid("category_parent_not_top_level", "Subcategorias ficam só um nível abaixo de uma categoria principal.")
	ErrParentInactive    = problem.Conflict("category_parent_inactive", "A categoria principal escolhida está desativada.")
	ErrDuplicateName     = problem.Conflict("category_duplicate_name", "Já existe uma categoria com esse nome no mesmo lugar.")
	ErrHasChildren       = problem.Conflict("category_has_children", "Esta categoria tem subcategorias. Mova ou exclua as subcategorias antes.")
	ErrInUse             = problem.Conflict("category_in_use", "Esta categoria já tem lançamentos e não pode ser excluída. Desative-a para tirá-la das opções.")
)

// Category is a node of the tree; Children is filled only for top level categories.
type Category struct {
	ID       int64
	ParentID *int64
	Name     string
	Icon     string
	Active   bool
	Children []Category
}

// NewCategory is the input to create a category; ParentID nil means top level.
type NewCategory struct {
	Name     string
	ParentID *int64
	Icon     string
}

// Changes holds optional edits; nil fields stay unchanged.
type Changes struct {
	Name   *string
	Icon   *string
	Active *bool
}

// Move is the destination of a move; ParentID nil promotes to top level, which needs an icon.
type Move struct {
	ParentID *int64
	Icon     string
}

// Service reads and changes the category tree.
type Service struct {
	conn *sql.DB
	now  func() time.Time
}

// NewService creates the category service.
func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn, now: time.Now}
}

// List returns the tree in display order; without includeInactive it keeps only the launch options (RN-18).
func (s *Service) List(ctx context.Context, includeInactive bool) ([]Category, error) {
	rows, err := store.New(s.conn).ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return buildTree(rows, includeInactive), nil
}

// Get returns one category, with its children when it is a top level one.
func (s *Service) Get(ctx context.Context, id int64) (Category, error) {
	rows, err := store.New(s.conn).ListCategories(ctx)
	if err != nil {
		return Category{}, fmt.Errorf("list categories: %w", err)
	}
	return findInTree(buildTree(rows, true), id)
}

// Create adds a category at the end of its parent, recording who created it (RF-03).
func (s *Service) Create(ctx context.Context, userID int64, in NewCategory) (Category, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return Category{}, err
	}
	icon, err := cleanIcon(in.Icon, in.ParentID == nil)
	if err != nil {
		return Category{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		all, err := q.ListCategories(ctx)
		if err != nil {
			return err
		}
		if err := checkParent(all, in.ParentID, 0); err != nil {
			return err
		}
		if err := checkUniqueName(all, in.ParentID, name, 0); err != nil {
			return err
		}
		order, err := q.NextCategorySortOrder(ctx, nullID(in.ParentID))
		if err != nil {
			return err
		}
		now := s.now().UTC()
		row, err := q.CreateCategory(ctx, store.CreateCategoryParams{
			ParentID:  nullID(in.ParentID),
			Name:      name,
			Icon:      nullString(icon),
			SortOrder: order,
			CreatedBy: sql.NullInt64{Int64: userID, Valid: true},
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create category: %w", err)
		}
		id = row.ID
		return nil
	})
	if err != nil {
		return Category{}, err
	}
	return s.Get(ctx, id)
}

// Update renames, changes the icon or toggles the category; deactivation keeps the history (RN-18).
func (s *Service) Update(ctx context.Context, id int64, c Changes) (Category, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		all, err := q.ListCategories(ctx)
		if err != nil {
			return err
		}
		cur, ok := rowByID(all, id)
		if !ok {
			return ErrNotFound
		}
		parent := idPtr(cur.ParentID)
		next := updateParams(cur, s.now().UTC())
		if c.Name != nil {
			name, err := cleanName(*c.Name)
			if err != nil {
				return err
			}
			if err := checkUniqueName(all, parent, name, id); err != nil {
				return err
			}
			next.Name = name
		}
		if c.Icon != nil {
			icon, err := cleanIcon(*c.Icon, parent == nil)
			if err != nil {
				return err
			}
			next.Icon = nullString(icon)
		}
		if c.Active != nil {
			next.Active = *c.Active
		}
		return q.UpdateCategory(ctx, next)
	})
	if err != nil {
		return Category{}, err
	}
	return s.Get(ctx, id)
}

// Move places the category under another parent, at the end, or promotes it to top level.
func (s *Service) Move(ctx context.Context, id int64, m Move) (Category, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		all, err := q.ListCategories(ctx)
		if err != nil {
			return err
		}
		cur, ok := rowByID(all, id)
		if !ok {
			return ErrNotFound
		}
		next := updateParams(cur, s.now().UTC())
		if m.ParentID == nil {
			icon := m.Icon
			if icon == "" {
				icon = cur.Icon.String
			}
			icon, err = cleanIcon(icon, true)
			if err != nil {
				return err
			}
			next.Icon = nullString(icon)
		} else {
			if err := checkParent(all, m.ParentID, id); err != nil {
				return err
			}
			if hasChildren(all, id) {
				return ErrHasChildren
			}
		}
		if err := checkUniqueName(all, m.ParentID, cur.Name, id); err != nil {
			return err
		}
		order, err := q.NextCategorySortOrder(ctx, nullID(m.ParentID))
		if err != nil {
			return err
		}
		next.ParentID = nullID(m.ParentID)
		next.SortOrder = order
		return q.UpdateCategory(ctx, next)
	})
	if err != nil {
		return Category{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes an unused category; one with entries or subcategories stays (RN-18).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetCategory(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		children, err := q.CountCategoryChildren(ctx, sql.NullInt64{Int64: id, Valid: true})
		if err != nil {
			return err
		}
		if children > 0 {
			return ErrHasChildren
		}
		// Entry tables reference categories without ON DELETE, so SQLite refuses the delete.
		if err := q.DeleteCategory(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrInUse
			}
			return fmt.Errorf("delete category: %w", err)
		}
		return nil
	})
}

// cleanName trims and validates a category name.
func cleanName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrNameRequired
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return "", ErrNameTooLong
	}
	return name, nil
}

// cleanIcon validates an icon name, which top level categories must have.
func cleanIcon(raw string, topLevel bool) (string, error) {
	icon := strings.TrimSpace(raw)
	if icon == "" {
		if topLevel {
			return "", ErrIconRequired
		}
		return "", nil
	}
	if len(icon) > 40 || !iconPattern.MatchString(icon) {
		return "", ErrInvalidIcon
	}
	return icon, nil
}

// checkParent ensures parentID, when set, is an active top level category other than self.
func checkParent(all []store.Category, parentID *int64, self int64) error {
	if parentID == nil {
		return nil
	}
	p, ok := rowByID(all, *parentID)
	if !ok {
		return ErrParentNotFound
	}
	if p.ParentID.Valid || p.ID == self {
		return ErrParentNotTopLevel
	}
	if !p.Active {
		return ErrParentInactive
	}
	return nil
}

// checkUniqueName rejects a name already used by a sibling, ignoring case.
func checkUniqueName(all []store.Category, parentID *int64, name string, self int64) error {
	for _, c := range all {
		if c.ID != self && sameParent(c.ParentID, parentID) && strings.EqualFold(c.Name, name) {
			return ErrDuplicateName
		}
	}
	return nil
}

// sameParent compares a stored parent with a requested one.
func sameParent(stored sql.NullInt64, want *int64) bool {
	if want == nil {
		return !stored.Valid
	}
	return stored.Valid && stored.Int64 == *want
}

// hasChildren reports whether any category points to id as parent.
func hasChildren(all []store.Category, id int64) bool {
	for _, c := range all {
		if c.ParentID.Valid && c.ParentID.Int64 == id {
			return true
		}
	}
	return false
}

// rowByID finds a stored category by id.
func rowByID(all []store.Category, id int64) (store.Category, bool) {
	for _, c := range all {
		if c.ID == id {
			return c, true
		}
	}
	return store.Category{}, false
}

// updateParams copies a stored category into update parameters stamped with now.
func updateParams(c store.Category, now time.Time) store.UpdateCategoryParams {
	return store.UpdateCategoryParams{
		ParentID:  c.ParentID,
		Name:      c.Name,
		Icon:      c.Icon,
		Active:    c.Active,
		SortOrder: c.SortOrder,
		UpdatedAt: now,
		ID:        c.ID,
	}
}

// buildTree groups rows already sorted by level and order into the two level tree.
func buildTree(rows []store.Category, includeInactive bool) []Category {
	var tree []Category
	index := map[int64]int{}
	for _, r := range rows {
		if r.ParentID.Valid || (!includeInactive && !r.Active) {
			continue
		}
		index[r.ID] = len(tree)
		tree = append(tree, toCategory(r))
	}
	for _, r := range rows {
		if !r.ParentID.Valid || (!includeInactive && !r.Active) {
			continue
		}
		if i, ok := index[r.ParentID.Int64]; ok {
			tree[i].Children = append(tree[i].Children, toCategory(r))
		}
	}
	return tree
}

// findInTree returns the node with id from the tree.
func findInTree(tree []Category, id int64) (Category, error) {
	for _, c := range tree {
		if c.ID == id {
			return c, nil
		}
		for _, sub := range c.Children {
			if sub.ID == id {
				return sub, nil
			}
		}
	}
	return Category{}, ErrNotFound
}

// toCategory converts a stored row to the service model.
func toCategory(r store.Category) Category {
	c := Category{ID: r.ID, ParentID: idPtr(r.ParentID), Name: r.Name, Icon: r.Icon.String, Active: r.Active}
	if c.ParentID == nil {
		c.Children = []Category{}
	}
	return c
}

// idPtr converts a nullable id to a pointer.
func idPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

// nullID converts an optional id to its SQL form.
func nullID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

// nullString stores an empty string as NULL.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
