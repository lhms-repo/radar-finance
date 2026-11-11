package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/problem"
)

// errNoBody is returned when a write arrives without a JSON body.
var errNoBody = problem.Invalid("invalid_request", "Dados não enviados.")

// ListCategories returns the category tree (RF-50, RN-18).
func (a *API) ListCategories(ctx context.Context, req gen.ListCategoriesRequestObject) (gen.ListCategoriesResponseObject, error) {
	includeInactive := req.Params.IncludeInactive != nil && *req.Params.IncludeInactive
	tree, err := a.d.Categories.List(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListCategories200JSONResponse, 0, len(tree))
	for _, c := range tree {
		out = append(out, toCategory(c))
	}
	return out, nil
}

// CreateCategory adds a category or subcategory (RF-50).
func (a *API) CreateCategory(ctx context.Context, req gen.CreateCategoryRequestObject) (gen.CreateCategoryResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	c, err := a.d.Categories.Create(ctx, auth.UserID(ctx), category.NewCategory{
		Name: req.Body.Name, ParentID: req.Body.ParentId, Icon: deref(req.Body.Icon),
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateCategory201JSONResponse(toCategory(c)), nil
}

// UpdateCategory renames, changes the icon or toggles a category (RF-50, RN-18).
func (a *API) UpdateCategory(ctx context.Context, req gen.UpdateCategoryRequestObject) (gen.UpdateCategoryResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	c, err := a.d.Categories.Update(ctx, req.Id, category.Changes{
		Name: req.Body.Name, Icon: req.Body.Icon, Active: req.Body.Active,
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateCategory200JSONResponse(toCategory(c)), nil
}

// MoveCategory moves a category under another parent or to the top level (RF-50).
func (a *API) MoveCategory(ctx context.Context, req gen.MoveCategoryRequestObject) (gen.MoveCategoryResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	c, err := a.d.Categories.Move(ctx, req.Id, category.Move{ParentID: req.Body.ParentId, Icon: deref(req.Body.Icon)})
	if err != nil {
		return nil, err
	}
	return gen.MoveCategory200JSONResponse(toCategory(c)), nil
}

// DeleteCategory removes an unused category (RN-18).
func (a *API) DeleteCategory(ctx context.Context, req gen.DeleteCategoryRequestObject) (gen.DeleteCategoryResponseObject, error) {
	if err := a.d.Categories.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteCategory204Response{}, nil
}

// toCategory converts a category node to the API model.
func toCategory(c category.Category) gen.Category {
	out := gen.Category{Id: c.ID, ParentId: c.ParentID, Name: c.Name, Active: c.Active}
	if c.Icon != "" {
		out.Icon = &c.Icon
	}
	if c.ParentID == nil {
		children := make([]gen.Category, 0, len(c.Children))
		for _, sub := range c.Children {
			children = append(children, toCategory(sub))
		}
		out.Children = &children
	}
	return out
}

// deref returns the value of an optional string, or "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
