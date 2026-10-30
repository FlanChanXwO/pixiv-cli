// Package follow 提供 pixiv follow 命令，对用户执行关注与取消关注。
package follow

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var userRecordTypes = map[string]struct{}{string(target.NamespaceUser): {}}

type options struct {
	environment.Options
	input    *environment.State
	restrict string
	onError  string
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
}

// New 构造 `pixiv follow` 命令；关注与取消关注都要求已认证账号。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec)}
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
			if _, err := record.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.WrapUsage(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request session.ProxyRequest, id int64) error {
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
	cmd.Flags().StringVar(&opts.onError, "on-error", record.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newRemove() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "remove [USER_ID]",
		Short: "Unfollow a user",
		Args:  a.env.ActionInputArgs("pixiv follow remove [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := record.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.WrapUsage(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request session.ProxyRequest, id int64) error {
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
	cmd.Flags().StringVar(&opts.onError, "on-error", record.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) actionInvoker(cmd *cobra.Command, options environment.Options, invoke func(context.Context, session.ProxyRequest, int64) error) func(context.Context, int64) error {
	var request session.ProxyRequest
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := a.env.Request(cmd, options.ProxyOptions)
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
	resolved, err := target.Resolve(ctx, target.Input{Value: value}, target.Contract{
		Operation:   operation,
		DefaultType: string(target.NamespaceUser),
		Types: []target.TypeSpec{{
			Name:              string(target.NamespaceUser),
			ResultKind:        target.NamespaceUser,
			BareReferenceKind: pixiv.ReferenceKindUser,
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("user_id: %w", err)
	}
	return resolved.ID, nil
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
