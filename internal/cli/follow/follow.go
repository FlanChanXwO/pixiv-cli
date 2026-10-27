// Package follow owns Pixiv follow mutations.
package follow

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/resolver"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var userRecordTypes = map[string]struct{}{string(resolver.ResultKindUser): {}}

type options struct {
	command.Options
	input    *input.State
	restrict string
	onError  string
}

type commandRunner struct {
	env    command.Env
	runner pixivcmd.Runner
}

// New builds the actual `pixiv follow` group command.
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: pixivcmd.New(env.Out, exec)}
	cmd := &cobra.Command{Use: "follow", Short: "Manage followed users"}
	cmd.AddCommand(a.newAdd(), a.newRemove())
	env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) newAdd() *cobra.Command {
	opts := options{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [USER_ID]",
		Short: "Follow a user",
		Args:  a.env.ActionInputArgs("pixiv follow add [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFollowRestrict(opts.restrict); err != nil {
				return err
			}
			if _, err := input.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.UsageError(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request pixivcmd.Request, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: id, Restrict: pixiv.Restrict(opts.restrict)})
				})
			})
			if len(args) == 0 {
				return a.env.ConsumeActionRecords(cmd, opts.input, "follow_add", opts.onError, userRecordTypes, invoke)
			}
			id, err := resolveFollowUserID(cmd.Context(), args[0], "follow add")
			if err != nil {
				return err
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	cmd.Flags().StringVar(&opts.onError, "on-error", input.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newRemove() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "remove [USER_ID]",
		Short: "Unfollow a user",
		Args:  a.env.ActionInputArgs("pixiv follow remove [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := input.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.UsageError(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request pixivcmd.Request, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: id})
				})
			})
			if len(args) == 0 {
				return a.env.ConsumeActionRecords(cmd, opts.input, "follow_remove", opts.onError, userRecordTypes, invoke)
			}
			id, err := resolveFollowUserID(cmd.Context(), args[0], "follow remove")
			if err != nil {
				return err
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.onError, "on-error", input.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) actionInvoker(cmd *cobra.Command, options command.Options, invoke func(context.Context, pixivcmd.Request, int64) error) func(context.Context, int64) error {
	var request pixivcmd.Request
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := pixivcmd.BuildRequest(cmd, options)
			if err != nil {
				return err
			}
			request = resolved
			initialized = true
		}
		return invoke(ctx, request, id)
	}
}

func resolveFollowUserID(ctx context.Context, value, operation string) (int64, error) {
	target, err := resolver.Resolve(ctx, resolver.Input{Value: value}, resolver.Contract{
		Operation:   operation,
		DefaultType: string(resolver.ResultKindUser),
		Types: []resolver.TypeSpec{{
			Name:              string(resolver.ResultKindUser),
			ResultKind:        resolver.ResultKindUser,
			BareReferenceKind: pixiv.ReferenceKindUser,
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("user_id: %w", err)
	}
	return target.ID, nil
}

func validateFollowRestrict(value string) error {
	switch pixiv.Restrict(value) {
	case "", pixiv.RestrictPublic, pixiv.RestrictPrivate:
		return nil
	default:
		return sdk.NewError("pixiv", "follow add", sdk.InvalidArgument,
			sdk.WithDetail("restrict must be public or private"))
	}
}
