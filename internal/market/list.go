package market

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// List statuses: open while shopping, done once finished (RF-77).
const (
	ListOpen = "open"
	ListDone = "done"
)

// maxQuantityMilli caps a list quantity at ten thousand units or kg.
const maxQuantityMilli = 10_000_000

// Errors of the shopping lists.
var (
	ErrListNotFound     = problem.NotFound("list_not_found", "Lista não encontrada.")
	ErrListName         = problem.Invalid("list_name_invalid", "Informe o nome da lista, com até 60 caracteres.")
	ErrListDone         = problem.Conflict("list_done", "Essa lista já foi finalizada.")
	ErrListItemNotFound = problem.NotFound("list_item_not_found", "Item não encontrado.")
	ErrListQuantity     = problem.Invalid("list_quantity_invalid", "Informe a quantidade: unidades inteiras, ou kg com até três casas nos itens por peso.")
)

// ShoppingList is a list with its counts (RF-71).
type ShoppingList struct {
	ID          int64
	Name        string
	Done        bool
	CreatedAt   time.Time
	ItemCount   int64
	PickedCount int64
}

// ListItem is a product of a list with its quantity in thousandths and the picked mark.
type ListItem struct {
	ID            int64
	Product       Product
	QuantityMilli int64
	Picked        bool
	PickedByName  string
}

// ListDetail is a list with its items, not picked first.
type ListDetail struct {
	ShoppingList
	Items []ListItem
}

// Lists returns the lists, open ones first.
func (s *Service) Lists(ctx context.Context) ([]ShoppingList, error) {
	rows, err := store.New(s.conn).ListShoppingLists(ctx)
	if err != nil {
		return nil, fmt.Errorf("list lists: %w", err)
	}
	out := make([]ShoppingList, 0, len(rows))
	for _, r := range rows {
		out = append(out, ShoppingList{ID: r.ID, Name: r.Name, Done: r.Status == ListDone, CreatedAt: r.CreatedAt, ItemCount: r.ItemCount, PickedCount: r.PickedCount})
	}
	return out, nil
}

// CreateList starts an empty list (RF-71).
func (s *Service) CreateList(ctx context.Context, userID int64, name string) (ShoppingList, error) {
	name, err := cleanListName(name)
	if err != nil {
		return ShoppingList{}, err
	}
	r, err := store.New(s.conn).CreateShoppingList(ctx, store.CreateShoppingListParams{Name: name, CreatedBy: userID, CreatedAt: s.now().UTC()})
	if err != nil {
		return ShoppingList{}, fmt.Errorf("create list: %w", err)
	}
	return ShoppingList{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt}, nil
}

// RenameList changes the name of a list.
func (s *Service) RenameList(ctx context.Context, id int64, name string) error {
	name, err := cleanListName(name)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getList(ctx, q, id); err != nil {
			return err
		}
		return q.RenameShoppingList(ctx, store.RenameShoppingListParams{Name: name, ID: id})
	})
}

// DeleteList removes a list and its items; a finished purchase keeps its expense.
func (s *Service) DeleteList(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getList(ctx, q, id); err != nil {
			return err
		}
		return q.DeleteShoppingList(ctx, id)
	})
}

// ListDetail returns a list with its items.
func (s *Service) ListDetail(ctx context.Context, id int64) (ListDetail, error) {
	q := store.New(s.conn)
	l, err := getList(ctx, q, id)
	if err != nil {
		return ListDetail{}, err
	}
	rows, err := q.ListItems(ctx, id)
	if err != nil {
		return ListDetail{}, fmt.Errorf("list items: %w", err)
	}
	out := ListDetail{ShoppingList: ShoppingList{ID: l.ID, Name: l.Name, Done: l.Status == ListDone, CreatedAt: l.CreatedAt}, Items: make([]ListItem, 0, len(rows))}
	for _, r := range rows {
		out.ItemCount++
		if r.Picked {
			out.PickedCount++
		}
		out.Items = append(out.Items, ListItem{
			ID:            r.ID,
			Product:       Product{ID: r.ProductID, Name: r.ProductName, Brand: r.Brand, ByWeight: r.ByWeight, SizeMilli: r.SizeMilli, Unit: Unit(r.Unit)},
			QuantityMilli: r.QuantityMilli, Picked: r.Picked, PickedByName: r.PickedByName.String,
		})
	}
	return out, nil
}

// AddItem puts a product on an open list; the same product again adds to its quantity (RF-71).
func (s *Service) AddItem(ctx context.Context, listID, productID, qtyMilli int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := openList(ctx, q, listID); err != nil {
			return err
		}
		p, err := getProduct(ctx, q, productID)
		if err != nil {
			return err
		}
		existing, err := q.ListItemByProduct(ctx, store.ListItemByProductParams{ListID: listID, ProductID: productID})
		switch {
		case err == nil:
			if err := checkQuantity(p, existing.QuantityMilli+qtyMilli); err != nil || qtyMilli <= 0 {
				return ErrListQuantity
			}
			return q.SetListItemQuantity(ctx, store.SetListItemQuantityParams{QuantityMilli: existing.QuantityMilli + qtyMilli, ID: existing.ID})
		case errors.Is(err, sql.ErrNoRows):
			if err := checkQuantity(p, qtyMilli); err != nil {
				return err
			}
			_, err := q.CreateListItem(ctx, store.CreateListItemParams{ListID: listID, ProductID: productID, QuantityMilli: qtyMilli, CreatedAt: s.now().UTC()})
			return err
		default:
			return err
		}
	})
}

// UpdateItem changes the quantity of an item.
func (s *Service) UpdateItem(ctx context.Context, itemID, qtyMilli int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		it, err := getItem(ctx, q, itemID)
		if err != nil {
			return err
		}
		if err := openList(ctx, q, it.ListID); err != nil {
			return err
		}
		p, err := getProduct(ctx, q, it.ProductID)
		if err != nil {
			return err
		}
		if err := checkQuantity(p, qtyMilli); err != nil {
			return err
		}
		return q.SetListItemQuantity(ctx, store.SetListItemQuantityParams{QuantityMilli: qtyMilli, ID: itemID})
	})
}

// SetPicked marks or unmarks an item as picked, keeping who picked it (RF-71).
func (s *Service) SetPicked(ctx context.Context, userID, itemID int64, picked bool) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		it, err := getItem(ctx, q, itemID)
		if err != nil {
			return err
		}
		if err := openList(ctx, q, it.ListID); err != nil {
			return err
		}
		by := sql.NullInt64{Int64: userID, Valid: picked}
		return q.SetListItemPicked(ctx, store.SetListItemPickedParams{Picked: picked, PickedBy: by, ID: itemID})
	})
}

// RemoveItem takes an item off an open list.
func (s *Service) RemoveItem(ctx context.Context, itemID int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		it, err := getItem(ctx, q, itemID)
		if err != nil {
			return err
		}
		if err := openList(ctx, q, it.ListID); err != nil {
			return err
		}
		return q.DeleteListItem(ctx, itemID)
	})
}

// checkQuantity requires whole units for packaged products and allows kg with decimals for products by weight (RF-66).
func checkQuantity(p Product, qtyMilli int64) error {
	if qtyMilli <= 0 || qtyMilli > maxQuantityMilli || (!p.ByWeight && qtyMilli%1_000 != 0) {
		return ErrListQuantity
	}
	return nil
}

// cleanListName trims and bounds a list name.
func cleanListName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return "", ErrListName
	}
	return name, nil
}

// getList reads one list.
func getList(ctx context.Context, q *store.Queries, id int64) (store.ShoppingList, error) {
	l, err := q.GetShoppingList(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrListNotFound
	}
	return l, err
}

// openList fails when the list is missing or already finished.
func openList(ctx context.Context, q *store.Queries, id int64) error {
	l, err := getList(ctx, q, id)
	if err != nil {
		return err
	}
	if l.Status == ListDone {
		return ErrListDone
	}
	return nil
}

// getItem reads one list item.
func getItem(ctx context.Context, q *store.Queries, id int64) (store.ListItem, error) {
	it, err := q.GetListItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrListItemNotFound
	}
	return it, err
}
