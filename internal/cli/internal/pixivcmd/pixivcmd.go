// Package pixivcmd owns the wiring and paged-read execution shared by Pixiv
// data commands: one request value, one SDK replay boundary, and the list
// runners that commit output only after it is safe to do so.

package pixivcmd

import (
	"context"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Request is one Pixiv data execution's transport and account selection. It
// carries no client and creates no second authentication path.
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Executor runs one read or mutation inside the account pool's replay boundary.
// The callback reports whether output or a mutation was already committed.
type Executor func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error

// Runner executes Pixiv SDK work with the ports selected by a command owner.
// It never owns account selection, retry policy, or presentation.
type Runner struct {
	out      io.Writer
	executor Executor
}

// New binds one command execution's output and replay port.
func New(out io.Writer, executor Executor) Runner {
	return Runner{out: out, executor: executor}
}

// Execute binds one request to the shared traversal contract. Logical paging,
// bookmark policy, and cursor handling stay with the command owner.
func (r Runner) Execute(request Request) pagination.Execute[*pixiv.Client] {
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return r.executor(ctx, request, attempt)
	}
}

// Read runs one read and returns its result only when the boundary succeeds.
// Partial results are never delivered.
func Read[T any](r Runner, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero, result T
	err := r.executor(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// Write runs one mutation. Every invoked attempt is committed because a
// transport error cannot prove the server rejected the request.
func (r Runner) Write(ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) error) error {
	return r.executor(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// BuildRequest reads the shared proxy flags into one execution request.
func BuildRequest(cmd *cobra.Command, opts command.Options) (Request, error) {
	proxy, err := command.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return Request{}, err
	}
	return Request{HTTPSProxyOverride: proxy}, nil
}

// CurrentUserID returns the identity of the opened local account. The CLI never
// falls back to an unknown user.
func CurrentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, errors.New("cannot determine current user id")
}
