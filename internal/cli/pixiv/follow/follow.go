// Package follow owns Pixiv follow mutations.
package follow

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var userRecordTypes = map[string]struct{}{"user": {}}

type options struct {
	deps.CommandOptions
	restrict string
	onError  string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv follow` group command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "follow", Short: "Manage followed users"}
	cmd.AddCommand(a.newAdd(), a.newRemove())
	data.BindNoInput(cmd)
	return cmd
}

func (a command) newAdd() *cobra.Command {
	opts := options{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [USER_ID]",
		Short: "Follow a user",
		Args:  a.data.ActionInputArgs("pixiv follow add [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: id, Restrict: pixiv.Restrict(opts.restrict)})
				})
			})
			if len(args) == 0 {
				return a.data.ConsumeActionRecords(cmd, "follow_add", opts.onError, userRecordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "user_id")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newRemove() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "remove [USER_ID]",
		Short: "Unfollow a user",
		Args:  a.data.ActionInputArgs("pixiv follow remove [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: id})
				})
			})
			if len(args) == 0 {
				return a.data.ConsumeActionRecords(cmd, "follow_remove", opts.onError, userRecordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "user_id")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) actionInvoker(cmd *cobra.Command, options deps.CommandOptions, invoke func(context.Context, deps.Request, int64) error) func(context.Context, int64) error {
	var request deps.Request
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := a.data.Request(cmd, options)
			if err != nil {
				return err
			}
			request = resolved
			initialized = true
		}
		return invoke(ctx, request, id)
	}
}
