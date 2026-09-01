package ascii2d

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// errSolverContextRequired protects the internal cache from a nil Context;
	// public ascii2d entry points validate the context before reaching it.
	errSolverContextRequired = errors.New("ascii2d: solver context is required")
	// errSolverClosed distinguishes lifecycle shutdown from a failed solve.
	errSolverClosed = errors.New("ascii2d: solver state cache is closed")
)

type solverCall struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	state   solverState
	err     error
}

type solverStateCache struct {
	mu     sync.Mutex
	state  *solverState
	active *solverCall
	closed bool
	now    func() time.Time
}

type solverSolveFunc func(context.Context) (solverState, error)

func newSolverStateCache() *solverStateCache {
	return &solverStateCache{now: time.Now}
}

func (c *solverStateCache) getOrSolve(ctx context.Context, solve solverSolveFunc) (solverState, error) {
	if ctx == nil {
		return solverState{}, errSolverContextRequired
	}
	if solve == nil {
		return solverState{}, ErrSolverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return solverState{}, err
	}
	if c == nil {
		return solverState{}, ErrSolverUnavailable
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return solverState{}, errSolverClosed
	}
	if c.state != nil {
		if !solverStateExpired(*c.state, c.currentTime()) {
			state := *c.state
			c.mu.Unlock()
			return state, nil
		}
		c.state = nil
	}

	call := c.active
	if call == nil || call.waiters == 0 {
		if call != nil {
			call.cancel()
		}
		callContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
		call = &solverCall{ctx: callContext, cancel: cancel, done: make(chan struct{}), waiters: 1}
		c.active = call
		go c.runSolver(call, solve)
	} else {
		call.waiters++
	}
	c.mu.Unlock()

	select {
	case <-call.done:
		return call.state, call.err
	case <-ctx.Done():
		c.unregisterSolverWaiter(call)
		return solverState{}, ctx.Err()
	}
}

func (c *solverStateCache) unregisterSolverWaiter(call *solverCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != call || call.waiters == 0 {
		return
	}
	call.waiters--
	if call.waiters == 0 {
		// 没有调用方继续等待时取消共享 solve；已有其他 waiter 时则保持
		// solve 活跃，避免单个请求的取消破坏同一 MCP 生命周期中的共享状态。
		call.cancel()
	}
}

func (c *solverStateCache) runSolver(call *solverCall, solve solverSolveFunc) {
	state, err := solve(call.ctx)

	c.mu.Lock()
	if c.closed {
		state = solverState{}
		err = errSolverClosed
	}
	call.state = state
	call.err = err
	if c.active == call {
		if !c.closed && err == nil && call.waiters > 0 && !solverStateExpired(state, c.currentTime()) {
			cached := state
			c.state = &cached
		}
		c.active = nil
	}
	close(call.done)
	c.mu.Unlock()
}

func (c *solverStateCache) invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.state = nil
	c.mu.Unlock()
}

func (c *solverStateCache) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.state = nil
		if c.active != nil {
			c.active.cancel()
		}
	}
	c.mu.Unlock()
}

func (c *solverStateCache) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func solverStateExpired(state solverState, now time.Time) bool {
	return state.hasExpiry && !now.Before(state.expiresAt)
}
