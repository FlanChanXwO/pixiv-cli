package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t fanboxTools) registerCreator(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_creator", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX creator profile."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorIn) (*mcp.CallToolResult, creatorOut, error) {
		return t.creatorHandler(ctx, input)
	})
}

type creatorOut struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	HasAdultContent   bool                 `json:"has_adult_content,omitempty"`
	IsFollowing       bool                 `json:"is_following,omitempty"`
	PlanFee           int                  `json:"plan_fee,omitempty"`
	HasSupportingPlan bool                 `json:"has_supporting_plan,omitempty"`
	Icon              *outputs.ResourceOut `json:"icon,omitempty"`
	Cover             *outputs.ResourceOut `json:"cover,omitempty"`
}

func (t fanboxTools) creatorHandler(ctx context.Context, input creatorIn) (callResult *mcp.CallToolResult, output creatorOut, handlerErr error) {
	empty := creatorOut{}
	if input.CreatorID == "" {
		return outputs.Result(true, "Error: creator_id is required"), empty, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), empty, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	creator, err := client.Creator(input.CreatorID).Get(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "Creator").Error()), empty, nil
	}
	creatorOut := creatorOut{
		ID:                creator.ID,
		Name:              creator.Name,
		HasAdultContent:   creator.HasAdultContent,
		IsFollowing:       creator.IsFollowing,
		PlanFee:           creator.PlanFee,
		HasSupportingPlan: creator.HasSupportingPlan,
		Icon:              outputs.ResourceOutFrom(record.FanboxImageResource(record.FanboxCreatorIcon, creator.ID, "", "", creator.Icon.URL)),
		Cover:             outputs.ResourceOutFrom(record.FanboxImageResource(record.FanboxCreatorCover, creator.ID, "", "", creator.Cover.URL)),
	}
	return outputs.Result(false, fmt.Sprintf("Retrieved creator %s.", creatorOut.ID)), creatorOut, nil
}

type creatorIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

func (t fanboxTools) registerCreatorPosts(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_creator_posts", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: creatorPostsInputSchema(), Description: "List posts from a FANBOX creator, optionally for one non-empty tag."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorPostsIn) (*mcp.CallToolResult, outputs.PostsOut, error) {
		return t.creatorPostsHandler(ctx, input)
	})
}

func (t fanboxTools) creatorPostsHandler(ctx context.Context, input creatorPostsIn) (callResult *mcp.CallToolResult, output outputs.PostsOut, handlerErr error) {
	out := outputs.PostsOut{Posts: []outputs.PostOut{}}
	plan, err := toolrun.ParseListPlan(input.ListIn)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	if input.CreatorID == "" {
		return outputs.Result(true, "Error: creator_id is required"), out, nil
	}
	if input.Tag != nil && *input.Tag == "" {
		return outputs.Result(true, "Error: tag must be non-empty"), out, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	creatorID := input.CreatorID
	return postList(ctx, input.Limit, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[*fanbox.Post], error) {
		if input.Tag != nil {
			page, err := client.Creator(creatorID).Posts(ctx, fanbox.PostsRequest{Tag: *input.Tag, Cursor: cursor})
			return page, report.Wrap(err, "TaggedPosts")
		}
		page, err := client.Creator(creatorID).Posts(ctx, fanbox.PostsRequest{Cursor: cursor})
		return page, report.Wrap(err, "CreatorPosts")
	})
}

type creatorPostsIn struct {
	Tag       *string `json:"tag,omitempty" jsonschema:"optional non-empty tag"`
	CreatorID string  `json:"creator_id" jsonschema:"required FANBOX creator id"`
	toolrun.ListIn
}

func creatorPostsInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[creatorPostsIn](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["tag"] = &jsonschema.Schema{Type: "string", Description: "Optional non-empty creator tag."}
	return schema
}
func (t fanboxTools) registerCreators(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_creators", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List supporting or following FANBOX creators."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorsIn) (*mcp.CallToolResult, creatorsOut, error) {
		return t.creatorsHandler(ctx, input)
	})
}

type creatorSummaryOut struct {
	ID   string               `json:"id"`
	Name string               `json:"name,omitempty"`
	Icon *outputs.ResourceOut `json:"icon,omitempty"`
}

type creatorsOut struct {
	Creators   []creatorSummaryOut      `json:"creators"`
	Pagination pagination.PaginationOut `json:"pagination"`
}

func (t fanboxTools) creatorsHandler(ctx context.Context, input creatorsIn) (callResult *mcp.CallToolResult, output creatorsOut, handlerErr error) {
	creatorsOut := creatorsOut{Creators: []creatorSummaryOut{}}
	plan, err := toolrun.ParseListPlan(input.ListIn)
	kind := fanbox.CreatorRelation(input.Relation)
	if input.Relation == "" {
		kind = fanbox.CreatorSupporting
	}
	if err == nil && kind != fanbox.CreatorSupporting && kind != fanbox.CreatorFollowing {
		err = errors.New("relation must be one of: supporting, following")
	}
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), creatorsOut, nil
	}
	lease, openErr := t.openClient(ctx)
	if openErr != nil {
		return outputs.Result(true, "Error: "+report.Wrap(openErr, "").Error()), creatorsOut, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	items, hasMore, fetchErr := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[*fanbox.Creator], error) {
		return client.Creators(ctx, fanbox.CreatorsRequest{Relation: kind, Cursor: cursor})
	})
	if fetchErr != nil {
		return outputs.Result(true, "Error: "+report.Wrap(fetchErr, "Creators").Error()), creatorsOut, nil
	}
	for _, item := range items {
		creatorsOut.Creators = append(creatorsOut.Creators, creatorSummaryOut{ID: item.ID})
	}
	creatorsOut.Pagination = pagination.ListPagination(plan, input.Limit, len(creatorsOut.Creators), hasMore)
	return outputs.Result(false, fmt.Sprintf("Retrieved %d creators.", len(creatorsOut.Creators))), creatorsOut, nil
}

type creatorsIn struct {
	Relation string `json:"relation,omitempty" jsonschema:"supporting or following"`
	toolrun.ListIn
}

func (t fanboxTools) registerCreatorTags(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_creator_tags", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List tags used by a FANBOX creator."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorTagsIn) (*mcp.CallToolResult, creatorTagsOut, error) {
		return t.creatorTagsHandler(ctx, input)
	})
}

type tagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type creatorTagsOut struct {
	Tags []tagOut `json:"tags"`
}

func (t fanboxTools) creatorTagsHandler(ctx context.Context, input creatorTagsIn) (callResult *mcp.CallToolResult, output creatorTagsOut, handlerErr error) {
	creatorTagsOut := creatorTagsOut{Tags: []tagOut{}}
	if input.CreatorID == "" {
		return outputs.Result(true, "Error: creator_id is required"), creatorTagsOut, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), creatorTagsOut, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	tags, err := client.Creator(input.CreatorID).Tags(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "CreatorTags").Error()), creatorTagsOut, nil
	}
	for _, tag := range tags {
		creatorTagsOut.Tags = append(creatorTagsOut.Tags, tagOut{Name: tag.Name, URL: tag.URL})
	}
	return outputs.Result(false, fmt.Sprintf("Retrieved %d tags.", len(creatorTagsOut.Tags))), creatorTagsOut, nil
}

type creatorTagsIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

func (t fanboxTools) registerFeed(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_feed", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: feedInputSchema(), Description: "Browse the FANBOX home or supporting feed."}, func(ctx context.Context, request *mcp.CallToolRequest, input feedIn) (*mcp.CallToolResult, outputs.PostsOut, error) {
		return t.feedHandler(ctx, input)
	})
}

func (t fanboxTools) feedHandler(ctx context.Context, input feedIn) (callResult *mcp.CallToolResult, output outputs.PostsOut, handlerErr error) {
	out := outputs.PostsOut{Posts: []outputs.PostOut{}}
	plan, err := toolrun.ParseListPlan(input.ListIn)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	if input.Source != "home" && input.Source != "supporting" {
		return outputs.Result(true, "Error: source must be home or supporting"), out, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	return postList(ctx, input.Limit, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[*fanbox.Post], error) {
		if input.Source == "supporting" {
			page, err := client.Supporting(ctx, fanbox.FeedRequest{Cursor: cursor})
			return page, report.Wrap(err, "Supporting")
		}
		page, err := client.Home(ctx, fanbox.FeedRequest{Cursor: cursor})
		return page, report.Wrap(err, "Home")
	})
}

type feedIn struct {
	Source string `json:"source" jsonschema:"required: home or supporting"`
	toolrun.ListIn
}

func feedInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[feedIn](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["source"].Enum = []any{"home", "supporting"}
	return schema
}
func (t fanboxTools) registerPost(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_post", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX post."}, func(ctx context.Context, request *mcp.CallToolRequest, input postIn) (*mcp.CallToolResult, outputs.PostOut, error) {
		return t.postHandler(ctx, input)
	})
}

func (t fanboxTools) postHandler(ctx context.Context, input postIn) (callResult *mcp.CallToolResult, output outputs.PostOut, handlerErr error) {
	out := outputs.PostOut{Assets: []outputs.PostAssetOut{}}
	if input.PostID == "" {
		return outputs.Result(true, "Error: post_id is required"), out, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	post, err := client.Post(input.PostID).Get(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "Post").Error()), out, nil
	}
	out = outputs.PostOutFrom(post)
	return outputs.Result(false, fmt.Sprintf("Retrieved post %s.", out.ID)), out, nil
}

type postIn struct {
	PostID string `json:"post_id" jsonschema:"required FANBOX post id"`
}

func (t fanboxTools) registerResolveURL(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_resolve_url", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}, Description: "Resolve a FANBOX page URL into a typed reference."}, func(ctx context.Context, request *mcp.CallToolRequest, input resolveURLIn) (*mcp.CallToolResult, resolveURLOut, error) {
		return t.resolveURLHandler(ctx, input)
	})
}

type resolveURLOut struct {
	Kind      string `json:"kind"`
	CreatorID string `json:"creator_id,omitempty"`
	PostID    string `json:"post_id,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

func (t fanboxTools) resolveURLHandler(ctx context.Context, input resolveURLIn) (callResult *mcp.CallToolResult, output resolveURLOut, handlerErr error) {
	result := resolveURLOut{}
	if strings.TrimSpace(input.URL) == "" {
		return outputs.Result(true, "Error: url is required"), result, nil
	}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), result, nil
	}
	defer closeClient(lease, &callResult)
	client := lease.Value()
	ref, err := client.ParseURL(input.URL)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), result, nil
	}
	switch ref := ref.(type) {
	case *fanbox.CreatorRef:
		result = resolveURLOut{Kind: "creator", CreatorID: ref.ID}
	case *fanbox.PostRef:
		result = resolveURLOut{Kind: "post", CreatorID: ref.CreatorID, PostID: ref.ID}
	case *fanbox.CreatorPostsRef:
		result = resolveURLOut{Kind: "creator_posts", CreatorID: ref.ID}
		if ref.Tag != "" {
			result.Kind = "tag"
			result.Tag = ref.Tag
		}
	}
	return outputs.Result(false, fmt.Sprintf("Resolved URL as %s.", result.Kind)), result, nil
}

type resolveURLIn struct {
	URL string `json:"url" jsonschema:"required FANBOX page URL"`
}

// postList 让 traversal 负责分页，同时保留 FANBOX 自己的列表 envelope。
func postList(ctx context.Context, limit *int, plan pagination.PagePlan, fetch func(context.Context, sdk.Cursor) (sdk.Page[*fanbox.Post], error)) (*mcp.CallToolResult, outputs.PostsOut, error) {
	out := outputs.PostsOut{Posts: []outputs.PostOut{}}
	items, more, err := collectPages(ctx, plan, fetch)
	if err != nil {
		return outputs.Result(true, "Error: "+report.Wrap(err, "").Error()), out, nil
	}
	for _, post := range items {
		out.Posts = append(out.Posts, outputs.PostOutFrom(post))
	}
	out.Pagination = pagination.ListPagination(plan, limit, len(out.Posts), more)
	return outputs.Result(false, fmt.Sprintf("Retrieved %d posts.", len(out.Posts))), out, nil
}

func collectPages[T any](ctx context.Context, plan pagination.PagePlan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) ([]T, bool, error) {
	items, result, err := pagination.CollectPagesFrom(ctx, pagination.PagePlan{Skip: plan.Skip, Limit: plan.Limit, OneBatch: plan.OneBatch}, sdk.Cursor{}, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		if err := ctx.Err(); err != nil {
			return nil, sdk.Cursor{}, err
		}
		page, err := fetch(ctx, cursor)
		return page.Items, page.Cursor, err
	})
	// 共享 traversal 保留 cursor 诊断，FANBOX 的线上输出不暴露 cursor。
	if err != nil && strings.HasPrefix(err.Error(), "pagination cursor repeated: ") {
		err = &paginationCycleError{cause: err}
	}
	return items, result.HasMore, err
}

type paginationCycleError struct{ cause error }

// Error 返回固定的 cursor 循环错误文本。
func (e *paginationCycleError) Error() string { return "pagination cursor repeated" }

// Unwrap 返回触发循环的原始错误，保留 errors.Is/As 分类。
func (e *paginationCycleError) Unwrap() error { return e.cause }
