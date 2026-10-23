// fanbox posts tools.

package tools

import (
	"context"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
)

// registerCreator 注册 fanbox_creator。
func (t fanboxTools) registerCreator(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_creator", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX creator profile."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorIn) (*mcp.CallToolResult, creatorOut, error) {
		return t.creatorHandler(ctx, input)
	})
}

type creatorOut struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	HasAdultContent   bool                  `json:"has_adult_content,omitempty"`
	IsFollowing       bool                  `json:"is_following,omitempty"`
	PlanFee           int                   `json:"plan_fee,omitempty"`
	HasSupportingPlan bool                  `json:"has_supporting_plan,omitempty"`
	Icon              *toolhost.ResourceOut `json:"icon,omitempty"`
	Cover             *toolhost.ResourceOut `json:"cover,omitempty"`
}

func (t fanboxTools) creatorHandler(ctx context.Context, input creatorIn) (*mcp.CallToolResult, creatorOut, error) {
	empty := creatorOut{}
	if input.CreatorID == "" {
		return toolhost.Result(empty, true, "Error: creator_id is required"), empty, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	defer lease.Close()
	client := lease.Value()
	creator, err := client.Creator(ctx, fanbox.CreatorRequest{CreatorID: input.CreatorID})
	if err != nil {
		return toolhost.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	creatorOut := creatorOut{
		ID:                creator.ID,
		Name:              creator.Name,
		HasAdultContent:   creator.HasAdultContent,
		IsFollowing:       creator.IsFollowing,
		PlanFee:           creator.PlanFee,
		HasSupportingPlan: creator.HasSupportingPlan,
		Icon:              toolhost.ResourceOutFrom(creator.Icon.Resource),
		Cover:             toolhost.ResourceOutFrom(creator.Cover.Resource),
	}
	return toolhost.Result(creatorOut, false, fmt.Sprintf("Retrieved creator %s.", creatorOut.ID)), creatorOut, nil
}

type creatorIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

// registerCreatorposts 注册 fanbox_creator_posts。
func (t fanboxTools) registerCreatorposts(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_creator_posts", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: creatorpostsInputSchema(), Description: "List posts from a FANBOX creator, optionally for one non-empty tag."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorpostsIn) (*mcp.CallToolResult, toolhost.PostsOut, error) {
		return t.creatorpostsHandler(ctx, input)
	})
}

func (t fanboxTools) creatorpostsHandler(ctx context.Context, input creatorpostsIn) (*mcp.CallToolResult, toolhost.PostsOut, error) {
	out := toolhost.PostsOut{Posts: []toolhost.PostOut{}}
	plan, err := toolhost.ParseListPlan(input.ListIn)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.CreatorID == "" {
		return toolhost.Result(out, true, "Error: creator_id is required"), out, nil
	}
	if input.Tag != nil && *input.Tag == "" {
		return toolhost.Result(out, true, "Error: tag must be non-empty"), out, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	creatorID := input.CreatorID
	return toolhost.PostList(ctx, t.app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if input.Tag != nil {
			return client.TaggedPosts(ctx, fanbox.TaggedPostsRequest{CreatorID: creatorID, Tag: *input.Tag, Cursor: cursor})
		}
		return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
	})
}

type creatorpostsIn struct {
	Tag       *string `json:"tag,omitempty" jsonschema:"optional non-empty tag"`
	CreatorID string  `json:"creator_id" jsonschema:"required FANBOX creator id"`
	toolhost.ListIn
}

func creatorpostsInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[creatorpostsIn](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["tag"] = &jsonschema.Schema{Type: "string", Description: "Optional non-empty creator tag."}
	return schema
}

// registerCreators 注册 fanbox_creators。
func (t fanboxTools) registerCreators(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_creators", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List supporting or following FANBOX creators."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatorsIn) (*mcp.CallToolResult, creatorsOut, error) {
		return t.creatorsHandler(ctx, input)
	})
}

type creatorSummaryOut struct {
	ID   string                `json:"id"`
	Name string                `json:"name,omitempty"`
	Icon *toolhost.ResourceOut `json:"icon,omitempty"`
}

type creatorsOut struct {
	Creators   []creatorSummaryOut    `json:"creators"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

func (t fanboxTools) creatorsHandler(ctx context.Context, input creatorsIn) (*mcp.CallToolResult, creatorsOut, error) {
	creatorsOut := creatorsOut{Creators: []creatorSummaryOut{}}
	plan, err := toolhost.ParseListPlan(input.ListIn)
	kind := fanbox.CreatorListKind(input.Kind)
	if input.Kind == "" {
		kind = fanbox.CreatorListSupporting
	}
	if err == nil && kind != fanbox.CreatorListSupporting && kind != fanbox.CreatorListFollowing {
		err = errors.New("kind must be one of: supporting, following")
	}
	if err != nil {
		return toolhost.Result(creatorsOut, true, "Error: "+err.Error()), creatorsOut, nil
	}
	lease, openErr := t.app.OpenClient(ctx)
	if openErr != nil {
		return toolhost.Result(creatorsOut, true, "Error: "+openErr.Error()), creatorsOut, nil
	}
	defer lease.Close()
	client := lease.Value()
	items, hasMore, fetchErr := toolhost.CollectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.CreatorSummary], error) {
		return client.Creators(ctx, fanbox.CreatorsRequest{Kind: kind, Cursor: cursor})
	})
	if fetchErr != nil {
		return toolhost.Result(creatorsOut, true, "Error: "+fetchErr.Error()), creatorsOut, nil
	}
	for _, item := range items {
		creatorsOut.Creators = append(creatorsOut.Creators, creatorSummaryOut{ID: item.ID, Name: item.Name, Icon: toolhost.ResourceOutFrom(item.Icon.Resource)})
	}
	creatorsOut.Pagination = toolhost.ListPagination(plan, input.Limit, len(creatorsOut.Creators), hasMore)
	return toolhost.Result(creatorsOut, false, fmt.Sprintf("Retrieved %d creators.", len(creatorsOut.Creators))), creatorsOut, nil
}

type creatorsIn struct {
	Kind string `json:"kind,omitempty" jsonschema:"supporting or following"`
	toolhost.ListIn
}

// registerCreatortags 注册 fanbox_creator_tags。
func (t fanboxTools) registerCreatortags(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_creator_tags", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List tags used by a FANBOX creator."}, func(ctx context.Context, request *mcp.CallToolRequest, input creatortagsIn) (*mcp.CallToolResult, creatortagsOut, error) {
		return t.creatortagsHandler(ctx, input)
	})
}

type tagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type creatortagsOut struct {
	Tags []tagOut `json:"tags"`
}

func (t fanboxTools) creatortagsHandler(ctx context.Context, input creatortagsIn) (*mcp.CallToolResult, creatortagsOut, error) {
	creatortagsOut := creatortagsOut{Tags: []tagOut{}}
	if input.CreatorID == "" {
		return toolhost.Result(creatortagsOut, true, "Error: creator_id is required"), creatortagsOut, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(creatortagsOut, true, "Error: "+err.Error()), creatortagsOut, nil
	}
	defer lease.Close()
	client := lease.Value()
	tags, err := client.CreatorTags(ctx, fanbox.CreatorTagsRequest{CreatorID: input.CreatorID})
	if err != nil {
		return toolhost.Result(creatortagsOut, true, "Error: "+err.Error()), creatortagsOut, nil
	}
	for _, tag := range tags {
		creatortagsOut.Tags = append(creatortagsOut.Tags, tagOut{Name: tag.Name, URL: tag.URL})
	}
	return toolhost.Result(creatortagsOut, false, fmt.Sprintf("Retrieved %d tags.", len(creatortagsOut.Tags))), creatortagsOut, nil
}

type creatortagsIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

// registerFeed 注册 fanbox_feed。
func (t fanboxTools) registerFeed(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_feed", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: feedInputSchema(), Description: "Browse the FANBOX home or supporting feed."}, func(ctx context.Context, request *mcp.CallToolRequest, input feedIn) (*mcp.CallToolResult, toolhost.PostsOut, error) {
		return t.feedHandler(ctx, input)
	})
}

func (t fanboxTools) feedHandler(ctx context.Context, input feedIn) (*mcp.CallToolResult, toolhost.PostsOut, error) {
	out := toolhost.PostsOut{Posts: []toolhost.PostOut{}}
	plan, err := toolhost.ParseListPlan(input.ListIn)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.Source != "home" && input.Source != "supporting" {
		return toolhost.Result(out, true, "Error: source must be home or supporting"), out, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	return toolhost.PostList(ctx, t.app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if input.Source == "supporting" {
			return client.Supporting(ctx, fanbox.SupportingRequest{Cursor: cursor})
		}
		return client.Home(ctx, fanbox.HomeRequest{Cursor: cursor})
	})
}

type feedIn struct {
	Source string `json:"source" jsonschema:"required: home or supporting"`
	toolhost.ListIn
}

func feedInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[feedIn](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["source"].Enum = []any{"home", "supporting"}
	return schema
}

// registerPost 注册 fanbox_post。
func (t fanboxTools) registerPost(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_post", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX post."}, func(ctx context.Context, request *mcp.CallToolRequest, input postIn) (*mcp.CallToolResult, toolhost.PostOut, error) {
		return t.postHandler(ctx, input)
	})
}

func (t fanboxTools) postHandler(ctx context.Context, input postIn) (*mcp.CallToolResult, toolhost.PostOut, error) {
	out := toolhost.PostOut{Assets: []toolhost.PostAssetOut{}}
	if input.PostID == "" {
		return toolhost.Result(out, true, "Error: post_id is required"), out, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	post, err := client.Post(ctx, fanbox.PostRequest{PostID: input.PostID})
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	out = toolhost.PostOutFrom(post)
	return toolhost.Result(out, false, fmt.Sprintf("Retrieved post %s.", out.ID)), out, nil
}

type postIn struct {
	PostID string `json:"post_id" jsonschema:"required FANBOX post id"`
}

// registerResolveurl 注册 fanbox_resolve_url。
func (t fanboxTools) registerResolveurl(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "fanbox_resolve_url", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}, Description: "Resolve a FANBOX page URL into a typed reference."}, func(ctx context.Context, request *mcp.CallToolRequest, input resolveurlIn) (*mcp.CallToolResult, resolveurlOut, error) {
		return t.resolveurlHandler(ctx, input)
	})
}

type resolveurlOut struct {
	Kind      string `json:"kind"`
	CreatorID string `json:"creator_id,omitempty"`
	PostID    string `json:"post_id,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

func (t fanboxTools) resolveurlHandler(ctx context.Context, input resolveurlIn) (*mcp.CallToolResult, resolveurlOut, error) {
	result := resolveurlOut{}
	if strings.TrimSpace(input.URL) == "" {
		return toolhost.Result(result, true, "Error: url is required"), result, nil
	}
	lease, err := t.app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(result, true, "Error: "+err.Error()), result, nil
	}
	defer lease.Close()
	client := lease.Value()
	ref, err := client.ResolveURL(ctx, fanbox.ResolveURLRequest{RawURL: input.URL})
	if err != nil {
		return toolhost.Result(result, true, "Error: "+err.Error()), result, nil
	}
	result = resolveurlOut{Kind: string(ref.Kind), CreatorID: ref.CreatorID, PostID: ref.PostID, Tag: ref.Tag}
	return toolhost.Result(result, false, fmt.Sprintf("Resolved URL as %s.", result.Kind)), result, nil
}

type resolveurlIn struct {
	URL string `json:"url" jsonschema:"required FANBOX page URL"`
}
