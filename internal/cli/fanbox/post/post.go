// Package post owns FANBOX content listing and post presentation command.
package post

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

type commandRunner struct {
	env  command.Env
	open func() (*session.Fanbox, error)
}

// Commands returns FANBOX content leaf commands for the root FANBOX group.
func Commands(env command.Env, open func() (*session.Fanbox, error)) []*cobra.Command {
	a := commandRunner{env: env, open: open}
	leaves := []*cobra.Command{
		a.newCreatorsCommand(),
		a.newPostsCommand(),
		a.newTagsCommand(),
		a.newHomeCommand(),
		a.newSupportingCommand(),
		a.newPostCommand(),
	}
	for _, cmd := range leaves {
		command.Bind(cmd, command.FanboxData())
	}
	return leaves
}

func (a commandRunner) newCreatorsCommand() *cobra.Command {
	var opts fanboxcmd.Options
	kind := string(fanbox.CreatorListSupporting)
	cmd := &cobra.Command{
		Use:   "creators",
		Short: "List supporting or following FANBOX creators",
		Args:  a.env.ExactArgs(0, "pixiv fanbox creators --kind supporting|following"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runCreators(cmd, opts, kind)
		},
	}
	fanboxcmd.BindListFlags(cmd, &opts)
	cmd.Flags().StringVar(&kind, "kind", kind, "creator list kind: supporting or following")
	a.env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) runCreators(cmd *cobra.Command, opts fanboxcmd.Options, rawKind string) error {
	kind := fanbox.CreatorListKind(rawKind)
	if kind != fanbox.CreatorListSupporting && kind != fanbox.CreatorListFollowing {
		return errors.New("kind must be one of: supporting, following")
	}
	plan, err := fanboxcmd.ParsePlan(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := fanboxcmd.JSONOut(cmd, opts, a.env.UsageError)
	if err != nil {
		return err
	}
	return fanboxcmd.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		fetch := func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.CreatorSummary], error) {
			return client.Creators(ctx, fanbox.CreatorsRequest{Kind: kind, Cursor: cursor})
		}
		return fanboxcmd.Run(cmd.Context(), a.env.Out, plan, "creators", jsonOut, opts.NDJSON, func(items []fanbox.CreatorSummary) error {
			return printCreators(a.env.Out, items)
		}, func(item fanbox.CreatorSummary) any { return item }, fetch)
	})
}

func (a commandRunner) newPostsCommand() *cobra.Command {
	var opts fanboxcmd.Options
	cmd := &cobra.Command{
		Use:   "posts SOURCE",
		Short: "List posts from a creator, tag, post, or FANBOX URL",
		Args:  a.env.ExactArgs(1, "pixiv fanbox posts SOURCE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runPosts(cmd, args, opts)
		},
	}
	fanboxcmd.BindListFlags(cmd, &opts)
	a.env.BindTextValue(cmd, 1, 1, 0)
	return cmd
}

func (a commandRunner) runPosts(cmd *cobra.Command, args []string, opts fanboxcmd.Options) error {
	plan, err := fanboxcmd.ParsePlan(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := fanboxcmd.JSONOut(cmd, opts, a.env.UsageError)
	if err != nil {
		return err
	}
	return fanboxcmd.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		fetch, err := PostsFetch(cmd.Context(), client, args[0])
		if err != nil {
			return err
		}
		return fanboxcmd.Run(cmd.Context(), a.env.Out, plan, "posts", jsonOut, opts.NDJSON, func(items []fanbox.Post) error {
			return printPosts(a.env.Out, items)
		}, func(item fanbox.Post) any { return item }, fetch)
	})
}

// PostsFetch 把 creator/post/URL 源解析为可分页的帖子获取函数。URL 经 ResolveURL
// 分类；纯数字源按 post ID 处理；其余按 creator ID 处理。FANBOX download 命令复用
// 同一解析，因此两条路由的源语义不会分叉。
func PostsFetch(ctx context.Context, client *fanbox.Client, source string) (func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error), error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		ref, err := client.ResolveURL(ctx, fanbox.ResolveURLRequest{RawURL: source})
		if err != nil {
			return nil, err
		}
		switch ref.Kind {
		case fanbox.ReferenceKindPost:
			return singlePostFetch(client, ref.PostID), nil
		case fanbox.ReferenceKindTag:
			creatorID, tag := ref.CreatorID, ref.Tag
			return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return client.TaggedPosts(ctx, fanbox.TaggedPostsRequest{CreatorID: creatorID, Tag: tag, Cursor: cursor})
			}, nil
		default:
			creatorID := ref.CreatorID
			return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
			}, nil
		}
	case isNumericID(source):
		return singlePostFetch(client, source), nil
	default:
		creatorID := source
		return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
			return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
		}, nil
	}
}

func singlePostFetch(client *fanbox.Client, postID string) func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error) {
	return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if !cursor.IsZero() {
			return sdk.Page[fanbox.Post]{Items: []fanbox.Post{}}, nil
		}
		post, err := client.Post(ctx, fanbox.PostRequest{PostID: postID})
		if err != nil {
			return sdk.Page[fanbox.Post]{}, err
		}
		return sdk.Page[fanbox.Post]{Items: []fanbox.Post{post}}, nil
	}
}

func isNumericID(source string) bool {
	if source == "" {
		return false
	}
	for _, r := range source {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type tagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

func (a commandRunner) newTagsCommand() *cobra.Command {
	var opts fanboxcmd.Options
	cmd := &cobra.Command{
		Use:   "tags CREATOR",
		Short: "List tags used by a FANBOX creator",
		Args:  a.env.ExactArgs(1, "pixiv fanbox tags CREATOR"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTags(cmd, args, opts)
		},
	}
	fanboxcmd.BindSingleFlags(cmd, &opts)
	a.env.BindTextValue(cmd, 1, 1, 0)
	return cmd
}

func (a commandRunner) runTags(cmd *cobra.Command, args []string, opts fanboxcmd.Options) error {
	jsonOut, err := fanboxcmd.JSONOut(cmd, opts, a.env.UsageError)
	if err != nil {
		return err
	}
	return fanboxcmd.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		tags, err := client.CreatorTags(cmd.Context(), fanbox.CreatorTagsRequest{CreatorID: args[0]})
		if err != nil {
			return err
		}
		out := make([]tagOut, 0, len(tags))
		for _, tag := range tags {
			out = append(out, tagOut{Name: tag.Name, URL: tag.URL})
		}
		if jsonOut {
			return a.env.PrintJSON(struct {
				Tags []tagOut `json:"tags"`
			}{Tags: out})
		}
		if opts.NDJSON {
			encoder := json.NewEncoder(a.env.Out)
			for _, item := range out {
				if err := encoder.Encode(item); err != nil {
					return err
				}
			}
			return nil
		}
		for _, tag := range out {
			fmt.Fprintf(a.env.Out, "tag:%s\n", tag.Name)
		}
		return nil
	})
}

func (a commandRunner) newHomeCommand() *cobra.Command {
	var opts fanboxcmd.Options
	cmd := &cobra.Command{
		Use:   "home",
		Short: "Browse the FANBOX home feed",
		Args:  a.env.ExactArgs(0, "pixiv fanbox home"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runFeed(cmd, opts, func(client *fanbox.Client) func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
					return client.Home(ctx, fanbox.HomeRequest{Cursor: cursor})
				}
			})
		},
	}
	fanboxcmd.BindListFlags(cmd, &opts)
	a.env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) newSupportingCommand() *cobra.Command {
	var opts fanboxcmd.Options
	cmd := &cobra.Command{
		Use:   "supporting",
		Short: "Browse posts from supporting creators",
		Args:  a.env.ExactArgs(0, "pixiv fanbox supporting"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runFeed(cmd, opts, func(client *fanbox.Client) func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
					return client.Supporting(ctx, fanbox.SupportingRequest{Cursor: cursor})
				}
			})
		},
	}
	fanboxcmd.BindListFlags(cmd, &opts)
	a.env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) runFeed(cmd *cobra.Command, opts fanboxcmd.Options, bind func(*fanbox.Client) func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error)) error {
	plan, err := fanboxcmd.ParsePlan(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := fanboxcmd.JSONOut(cmd, opts, a.env.UsageError)
	if err != nil {
		return err
	}
	return fanboxcmd.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		return fanboxcmd.Run(cmd.Context(), a.env.Out, plan, "posts", jsonOut, opts.NDJSON, func(items []fanbox.Post) error {
			return printPosts(a.env.Out, items)
		}, func(item fanbox.Post) any { return item }, bind(client))
	})
}

func (a commandRunner) newPostCommand() *cobra.Command {
	var opts fanboxcmd.Options
	cmd := &cobra.Command{
		Use:   "post POST_ID",
		Short: "Show one FANBOX post",
		Args:  a.env.ExactArgs(1, "pixiv fanbox post POST_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runPost(cmd, args, opts)
		},
	}
	fanboxcmd.BindSingleFlags(cmd, &opts)
	a.env.BindTextValue(cmd, 1, 1, 0)
	return cmd
}

func (a commandRunner) runPost(cmd *cobra.Command, args []string, opts fanboxcmd.Options) error {
	jsonOut, err := fanboxcmd.JSONOut(cmd, opts, a.env.UsageError)
	if err != nil {
		return err
	}
	return fanboxcmd.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		post, err := client.Post(cmd.Context(), fanbox.PostRequest{PostID: args[0]})
		if err != nil {
			return err
		}
		out := postOutFrom(post)
		if jsonOut {
			return a.env.PrintJSON(out)
		}
		if opts.NDJSON {
			return json.NewEncoder(a.env.Out).Encode(out)
		}
		return printPosts(a.env.Out, []fanbox.Post{post})
	})
}

type postOut struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	PublishedAt  string `json:"published_at"`
	CreatorID    string `json:"creator_id"`
	FeeRequired  int    `json:"fee_required,omitempty"`
	IsRestricted bool   `json:"is_restricted"`
	IsPinned     bool   `json:"is_pinned,omitempty"`
	CommentCount int    `json:"comment_count,omitempty"`
}

func postOutFrom(post fanbox.Post) postOut {
	published := ""
	if !post.PublishedAt.IsZero() {
		published = post.PublishedAt.UTC().Format(time.RFC3339)
	}
	return postOut{
		ID:           post.ID,
		Title:        post.Title,
		PublishedAt:  published,
		CreatorID:    post.CreatorID,
		FeeRequired:  post.FeeRequired,
		IsRestricted: post.IsRestricted,
		IsPinned:     post.IsPinned,
		CommentCount: post.CommentCount,
	}
}

func printCreators(out io.Writer, creators []fanbox.CreatorSummary) error {
	for _, creator := range creators {
		fmt.Fprintf(out, "id:%s", creator.ID)
		if creator.Name != "" {
			fmt.Fprintf(out, " name:%s", creator.Name)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func printPosts(out io.Writer, posts []fanbox.Post) error {
	for _, post := range posts {
		fmt.Fprintf(out, "id:%s title:%s published:%s restricted:%s\n",
			post.ID, post.Title, postOutFrom(post).PublishedAt, command.TextBool(post.IsRestricted))
	}
	return nil
}
