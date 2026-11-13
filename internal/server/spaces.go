package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/space"
)

// MoveToSpace moves an item of the active space, with its entries, to another space of the user (RF-83).
func (a *API) MoveToSpace(ctx context.Context, req gen.MoveToSpaceRequestObject) (gen.MoveToSpaceResponseObject, error) {
	if req.Body == nil {
		return nil, space.ErrUnknownKind
	}
	if err := a.d.Spaces.Move(ctx, space.Kind(req.Body.Kind), req.Body.ItemId, req.Id); err != nil {
		return nil, err
	}
	return gen.MoveToSpace204Response{}, nil
}

// optionalID leaves a zero id out of the API model.
func optionalID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
