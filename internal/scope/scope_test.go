package scope

import (
	"context"
	"errors"
	"testing"
)

// TestGroup reads the active group and refuses a context without one.
func TestGroup(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		want    int64
		wantErr error
	}{
		{name: "active group", ctx: WithGroup(context.Background(), 7), want: 7},
		{name: "no group", ctx: context.Background(), wantErr: ErrNoGroup},
		{name: "zero is no group", ctx: WithGroup(context.Background(), 0), wantErr: ErrNoGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Group(tt.ctx)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Group = %d, %v; want %d, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
