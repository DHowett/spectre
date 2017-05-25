package rayman

import (
	"context"

	"github.com/google/uuid"
)

type ID string

type key int

const (
	rayKey key = iota
	loggerKey
)

func newRayID() ID {
	return ID(uuid.New().String())
}

func ContextWithRay(ctx context.Context) context.Context {
	return context.WithValue(ctx, rayKey, newRayID())
}

func FromContext(ctx context.Context) (ID, bool) {
	id, ok := ctx.Value(rayKey).(ID)
	return id, ok
}
