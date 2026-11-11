package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/market"
)

// ListShoppingLists returns the lists, open ones first (RF-71).
func (a *API) ListShoppingLists(ctx context.Context, _ gen.ListShoppingListsRequestObject) (gen.ListShoppingListsResponseObject, error) {
	ls, err := a.d.Market.Lists(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListShoppingLists200JSONResponse, 0, len(ls))
	for _, l := range ls {
		out = append(out, toShoppingList(l))
	}
	return out, nil
}

// CreateShoppingList starts an empty list (RF-71).
func (a *API) CreateShoppingList(ctx context.Context, req gen.CreateShoppingListRequestObject) (gen.CreateShoppingListResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	l, err := a.d.Market.CreateList(ctx, auth.UserID(ctx), req.Body.Name)
	if err != nil {
		return nil, err
	}
	return gen.CreateShoppingList201JSONResponse(toShoppingList(l)), nil
}

// GetShoppingList returns a list with its items (RF-71).
func (a *API) GetShoppingList(ctx context.Context, req gen.GetShoppingListRequestObject) (gen.GetShoppingListResponseObject, error) {
	d, err := a.d.Market.ListDetail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	l := toShoppingList(d.ShoppingList)
	out := gen.GetShoppingList200JSONResponse{Id: l.Id, Name: l.Name, Done: l.Done, ItemCount: l.ItemCount, PickedCount: l.PickedCount, Items: make([]gen.ListItem, 0, len(d.Items))}
	for _, it := range d.Items {
		item := gen.ListItem{Id: it.ID, Product: toProduct(it.Product), QuantityMilli: it.QuantityMilli, Picked: it.Picked}
		if it.PickedByName != "" {
			name := it.PickedByName
			item.PickedByName = &name
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// RenameShoppingList renames a list.
func (a *API) RenameShoppingList(ctx context.Context, req gen.RenameShoppingListRequestObject) (gen.RenameShoppingListResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Market.RenameList(ctx, req.Id, req.Body.Name); err != nil {
		return nil, err
	}
	return gen.RenameShoppingList204Response{}, nil
}

// DeleteShoppingList removes a list.
func (a *API) DeleteShoppingList(ctx context.Context, req gen.DeleteShoppingListRequestObject) (gen.DeleteShoppingListResponseObject, error) {
	if err := a.d.Market.DeleteList(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteShoppingList204Response{}, nil
}

// AddListItem puts a product on a list (RF-71).
func (a *API) AddListItem(ctx context.Context, req gen.AddListItemRequestObject) (gen.AddListItemResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Market.AddItem(ctx, req.Id, req.Body.ProductId, req.Body.QuantityMilli); err != nil {
		return nil, err
	}
	return gen.AddListItem204Response{}, nil
}

// UpdateListItem changes the quantity of an item.
func (a *API) UpdateListItem(ctx context.Context, req gen.UpdateListItemRequestObject) (gen.UpdateListItemResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Market.UpdateItem(ctx, req.Id, req.Body.QuantityMilli); err != nil {
		return nil, err
	}
	return gen.UpdateListItem204Response{}, nil
}

// RemoveListItem takes an item off a list.
func (a *API) RemoveListItem(ctx context.Context, req gen.RemoveListItemRequestObject) (gen.RemoveListItemResponseObject, error) {
	if err := a.d.Market.RemoveItem(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.RemoveListItem204Response{}, nil
}

// SetListItemPicked marks or unmarks an item as picked (RF-71).
func (a *API) SetListItemPicked(ctx context.Context, req gen.SetListItemPickedRequestObject) (gen.SetListItemPickedResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Market.SetPicked(ctx, auth.UserID(ctx), req.Id, req.Body.Picked); err != nil {
		return nil, err
	}
	return gen.SetListItemPicked204Response{}, nil
}

// toShoppingList converts a list to the API model.
func toShoppingList(l market.ShoppingList) gen.ShoppingList {
	return gen.ShoppingList{Id: l.ID, Name: l.Name, Done: l.Done, ItemCount: l.ItemCount, PickedCount: l.PickedCount}
}
