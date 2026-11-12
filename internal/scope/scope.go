// Package scope carries the active group of a request, so every service reads and writes only the data of
// that group (RF-02, RF-81). A request without a group fails closed instead of reading every group.
package scope

import (
	"context"

	"github.com/italogfx/radar-finance/internal/problem"
)

// ctxKey is the private type of the context key.
type ctxKey struct{}

// ErrNoGroup is returned when a service runs without an active group, which is a bug in the caller.
var ErrNoGroup = problem.Invalid("no_active_group", "Escolha um grupo para continuar.")

// WithGroup returns a context whose requests act on the given group.
func WithGroup(ctx context.Context, groupID int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, groupID)
}

// Group returns the active group of the request, or ErrNoGroup when there is none.
func Group(ctx context.Context) (int64, error) {
	id, _ := ctx.Value(ctxKey{}).(int64)
	if id <= 0 {
		return 0, ErrNoGroup
	}
	return id, nil
}
