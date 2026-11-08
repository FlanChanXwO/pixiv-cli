package cloudflare

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const logReason = "reason"

type solveCall struct {
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int // Protected by Client.mu; cancel only after the last waiter leaves.
	state   Clearance
	err     error
}

// Solve obtains or reuses clearance for a protected URL within the configured origin.
// Query parameters are preserved; the product owns the target's privacy policy.
// The product must detect a real challenge before invoking it and must scope the
// returned cookie to its validated product host. Canceling one waiter leaves a
// shared solve alive for the others; canceling all waiters stops that request.
func (c *Client) Solve(ctx context.Context, targetURL string) (Clearance, error) {
	if c.endpoint == "" {
		return Clearance{}, failure(operationSolve, sdk.ChallengeRequired, "FlareSolverr is not configured", nil)
	}
	target, err := httpURL(targetURL)
	if err != nil {
		return Clearance{}, err
	}
	if !sameOrigin(c.origin, target) {
		return Clearance{}, failure(operationSolve, sdk.InvalidArgument, "challenge target is outside the configured origin", nil)
	}
	for {
		if err := ctx.Err(); err != nil {
			return Clearance{}, failure(operationSolve, sdk.UpstreamUnavailable, "solver request canceled", err)
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return Clearance{}, failure(operationSolve, sdk.LocalStateError, "solver client is closed", nil)
		}
		if c.state != nil && !c.state.expired() {
			state := *c.state
			c.mu.Unlock()
			return state, nil
		}
		c.state = nil
		call := c.active
		if call != nil && call.waiters == 0 {
			// Join the canceled flight before starting another on the same browser session.
			c.mu.Unlock()
			select {
			case <-call.done:
				continue
			case <-ctx.Done():
				return Clearance{}, failure(operationSolve, sdk.UpstreamUnavailable, "solver request canceled", ctx.Err())
			}
		}
		if call == nil {
			// Preserve request values, but no single caller owns the shared cancellation.
			solveContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
			call = &solveCall{cancel: cancel, done: make(chan struct{}), waiters: 1}
			c.active = call
			go c.runSolve(solveContext, call, target.String())
		} else {
			call.waiters++
		}
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.state, call.err
		case <-ctx.Done():
			c.mu.Lock()
			call.waiters--
			if call.waiters == 0 {
				call.cancel()
			}
			c.mu.Unlock()
			return Clearance{}, failure(operationSolve, sdk.UpstreamUnavailable, "solver request canceled", ctx.Err())
		}
	}
}

func (c *Client) runSolve(ctx context.Context, call *solveCall, target string) {
	defer call.cancel()
	slog.DebugContext(ctx, "Starting Cloudflare solver")
	state, err := c.solve(ctx, target)
	if err == nil {
		slog.DebugContext(ctx, "Cloudflare clearance acquired")
	} else if ctx.Err() == nil {
		slog.WarnContext(ctx, "Cloudflare solver failed", logReason, sdk.ReasonOf(err))
	}
	c.mu.Lock()
	if c.closed {
		state, err = Clearance{}, failure(operationSolve, sdk.LocalStateError, "solver client is closed", err)
	}
	call.state, call.err = state, err
	if c.active == call {
		if err == nil && call.waiters > 0 {
			c.state = &state
		}
		c.active = nil
	}
	close(call.done) // Publish state and error before waking readers.
	c.mu.Unlock()
}

// Invalidate discards cached clearance after the product detects a new challenge.
func (c *Client) Invalidate() { c.mu.Lock(); c.state = nil; c.mu.Unlock() }

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if value.Scheme == "https" {
		return defaultHTTPSPort
	}
	return defaultHTTPPort
}
