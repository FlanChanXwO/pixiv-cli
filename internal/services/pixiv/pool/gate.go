package pool

import (
	"context"
	"errors"
)

// Gate serializes executions that may consume the same Pixiv refresh token.
// It is product account-pool state, not an MCP server global.
type Gate struct{ slots chan struct{} }

// NewGate creates a gate with one slot.
func NewGate() *Gate { return &Gate{slots: make(chan struct{}, 1)} }

// Acquire waits for the serialization slot and is paired with Release.
func (g *Gate) Acquire(ctx context.Context) error {
	if g == nil || g.slots == nil {
		return errors.New("pixiv rotation gate is not configured")
	}
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a slot acquired by Acquire.
func (g *Gate) Release() {
	if g != nil && g.slots != nil {
		<-g.slots
	}
}

// Run executes fn while holding the rotation serialization slot.
func (g *Gate) Run(ctx context.Context, fn func(context.Context) error) error {
	if g == nil || g.slots == nil {
		return errors.New("pixiv rotation gate is not configured")
	}
	if fn == nil {
		return errors.New("pixiv rotation gate function is nil")
	}
	if err := g.Acquire(ctx); err != nil {
		return err
	}
	defer g.Release()
	return fn(ctx)
}
