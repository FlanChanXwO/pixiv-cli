package fanbox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fanboxListIn 与 pixiv MCP 的 pageLimitIn 语义一致；Cursor 始终是 SDK 内部细节，
// 不出现在 MCP tool 输入或输出中。
type fanboxListIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

type fanboxListPlan struct {
	page     int
	limit    int
	oneBatch bool
	skip     int
}

type fanboxPaginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

func parseFanboxListPlan(input fanboxListIn) (fanboxListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return fanboxListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return fanboxListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return fanboxListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return fanboxListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return fanboxListPlan{page: *input.Page, limit: *input.Limit, skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return fanboxListPlan{page: 1, limit: -1, oneBatch: true}, nil
	}
	return fanboxListPlan{page: 1, limit: *input.Limit}, nil
}

func fanboxListPagination(plan fanboxListPlan, limit *int, returned int, hasMore bool) fanboxPaginationOut {
	out := fanboxPaginationOut{Page: plan.page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.page < math.MaxInt {
		next := plan.page + 1
		out.NextPage = &next
	}
	return out
}

// collectFanboxPages 跟随 sdk.Cursor 收集分页结果；失败时丢弃部分结果。
func collectFanboxPages[T any](ctx context.Context, plan fanboxListPlan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) ([]T, bool, error) {
	limit := plan.limit
	if limit < 0 {
		limit = 0
	}
	items := make([]T, 0)
	cursor := sdk.Cursor{}
	seen := make(map[string]struct{})
	skip := plan.skip
	returned := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		key := cursor.String()
		if _, exists := seen[key]; exists {
			return nil, false, errors.New("pagination cursor repeated")
		}
		seen[key] = struct{}{}
		page, err := fetch(ctx, cursor)
		if err != nil {
			return nil, false, err
		}
		batch := page.Items
		if skip >= len(batch) {
			skip -= len(batch)
			batch = nil
		} else if skip > 0 {
			batch = batch[skip:]
			skip = 0
		}
		if limit > 0 {
			remaining := limit - returned
			if len(batch) > remaining {
				batch = batch[:remaining]
			}
		}
		items = append(items, batch...)
		returned += len(batch)
		if limit > 0 && returned >= limit {
			return items, !page.Next.IsZero(), nil
		}
		if plan.oneBatch && (returned > 0 || page.Next.IsZero()) {
			return items, !page.Next.IsZero(), nil
		}
		if page.Next.IsZero() {
			return items, false, nil
		}
		cursor = page.Next
	}
}

// ---------------------------------------------------------------------------
// current user
// ---------------------------------------------------------------------------

type currentUserIn struct{}

type currentUserOut struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

func (a *App) currentUser(ctx context.Context, _ *mcp.CallToolRequest, _ currentUserIn) (*mcp.CallToolResult, currentUserOut, error) {
	out := currentUserOut{}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	user, err := client.CurrentUser(ctx, fanboxsdk.CurrentUserRequest{})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	out = currentUserOut{
		UserID:        user.UserID,
		DisplayName:   user.DisplayName,
		CreatorID:     user.CreatorID,
		CreatorStatus: user.CreatorStatus,
		IsCreator:     user.IsCreator,
	}
	return fanboxResult(out, false, fmt.Sprintf("Current FANBOX user %d.", out.UserID)), out, nil
}

// ---------------------------------------------------------------------------
// creator
// ---------------------------------------------------------------------------

type creatorIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

type fanboxResourceOut struct {
	URL            string            `json:"url"`
	Ref            string            `json:"ref"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
}

func fanboxResourceOutFrom(res sdk.Resource) *fanboxResourceOut {
	if res.Ref.IsZero() && res.URL == "" {
		return nil
	}
	return &fanboxResourceOut{URL: res.URL, Ref: res.Ref.String(), RequestHeaders: res.RequestHeaders}
}

type creatorOut struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	HasAdultContent   bool               `json:"has_adult_content,omitempty"`
	IsFollowing       bool               `json:"is_following,omitempty"`
	PlanFee           int                `json:"plan_fee,omitempty"`
	HasSupportingPlan bool               `json:"has_supporting_plan,omitempty"`
	Icon              *fanboxResourceOut `json:"icon,omitempty"`
	Cover             *fanboxResourceOut `json:"cover,omitempty"`
}

func (a *App) creator(ctx context.Context, _ *mcp.CallToolRequest, in creatorIn) (*mcp.CallToolResult, creatorOut, error) {
	out := creatorOut{}
	if in.CreatorID == "" {
		return fanboxResult(out, true, "Error: creator_id is required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	creator, err := client.Creator(ctx, fanboxsdk.CreatorRequest{CreatorID: in.CreatorID})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	out = creatorOut{
		ID:                creator.ID,
		Name:              creator.Name,
		HasAdultContent:   creator.HasAdultContent,
		IsFollowing:       creator.IsFollowing,
		PlanFee:           creator.PlanFee,
		HasSupportingPlan: creator.HasSupportingPlan,
		Icon:              fanboxResourceOutFrom(creator.Icon.Resource),
		Cover:             fanboxResourceOutFrom(creator.Cover.Resource),
	}
	return fanboxResult(out, false, fmt.Sprintf("Retrieved creator %s.", out.ID)), out, nil
}

// ---------------------------------------------------------------------------
// creators
// ---------------------------------------------------------------------------

type creatorsIn struct {
	Kind string `json:"kind,omitempty" jsonschema:"supporting or following"`
	fanboxListIn
}

type creatorSummaryOut struct {
	ID   string             `json:"id"`
	Name string             `json:"name,omitempty"`
	Icon *fanboxResourceOut `json:"icon,omitempty"`
}

type creatorsOut struct {
	Creators   []creatorSummaryOut `json:"creators"`
	Pagination fanboxPaginationOut `json:"pagination"`
}

func (a *App) creators(ctx context.Context, _ *mcp.CallToolRequest, in creatorsIn) (*mcp.CallToolResult, creatorsOut, error) {
	out := creatorsOut{Creators: []creatorSummaryOut{}}
	plan, err := parseFanboxListPlan(in.fanboxListIn)
	kind := fanboxsdk.CreatorListKind(in.Kind)
	if in.Kind == "" {
		kind = fanboxsdk.CreatorListSupporting
	}
	if err == nil && kind != fanboxsdk.CreatorListSupporting && kind != fanboxsdk.CreatorListFollowing {
		err = errors.New("kind must be one of: supporting, following")
	}
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	client, openErr := a.openClient(ctx)
	if openErr != nil {
		return fanboxResult(out, true, "Error: "+openErr.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	items, hasMore, fetchErr := collectFanboxPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanboxsdk.CreatorSummary], error) {
		return client.Creators(ctx, fanboxsdk.CreatorsRequest{Kind: kind, Cursor: cursor})
	})
	if fetchErr != nil {
		return fanboxResult(out, true, "Error: "+fetchErr.Error()), out, nil
	}
	for _, item := range items {
		out.Creators = append(out.Creators, creatorSummaryOut{ID: item.ID, Name: item.Name, Icon: fanboxResourceOutFrom(item.Icon.Resource)})
	}
	out.Pagination = fanboxListPagination(plan, in.Limit, len(out.Creators), hasMore)
	return fanboxResult(out, false, fmt.Sprintf("Retrieved %d creators.", len(out.Creators))), out, nil
}

// ---------------------------------------------------------------------------
// creator tags
// ---------------------------------------------------------------------------

type creatorTagsIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

type fanboxTagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type creatorTagsOut struct {
	Tags []fanboxTagOut `json:"tags"`
}

func (a *App) creatorTags(ctx context.Context, _ *mcp.CallToolRequest, in creatorTagsIn) (*mcp.CallToolResult, creatorTagsOut, error) {
	out := creatorTagsOut{Tags: []fanboxTagOut{}}
	if in.CreatorID == "" {
		return fanboxResult(out, true, "Error: creator_id is required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	tags, err := client.CreatorTags(ctx, fanboxsdk.CreatorTagsRequest{CreatorID: in.CreatorID})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	for _, tag := range tags {
		out.Tags = append(out.Tags, fanboxTagOut{Name: tag.Name, URL: tag.URL})
	}
	return fanboxResult(out, false, fmt.Sprintf("Retrieved %d tags.", len(out.Tags))), out, nil
}

// ---------------------------------------------------------------------------
// posts
// ---------------------------------------------------------------------------

type fanboxPostAssetOut struct {
	ID        string             `json:"id"`
	Kind      string             `json:"kind"`
	Name      string             `json:"name,omitempty"`
	Resource  *fanboxResourceOut `json:"resource,omitempty"`
	Thumbnail *fanboxResourceOut `json:"thumbnail,omitempty"`
}

type fanboxPostOut struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	PublishedAt   string               `json:"published_at"`
	CreatorID     string               `json:"creator_id"`
	FeeRequired   int                  `json:"fee_required,omitempty"`
	IsRestricted  bool                 `json:"is_restricted"`
	IsPinned      bool                 `json:"is_pinned,omitempty"`
	RestrictedFor int                  `json:"restricted_for,omitempty"`
	CommentCount  int                  `json:"comment_count,omitempty"`
	Assets        []fanboxPostAssetOut `json:"assets"`
}

func fanboxPostOutFrom(post fanboxsdk.Post) fanboxPostOut {
	published := ""
	if !post.PublishedAt.IsZero() {
		published = post.PublishedAt.UTC().Format(time.RFC3339)
	}
	out := fanboxPostOut{
		ID:            post.ID,
		Title:         post.Title,
		PublishedAt:   published,
		CreatorID:     post.CreatorID,
		FeeRequired:   post.FeeRequired,
		IsRestricted:  post.IsRestricted,
		IsPinned:      post.IsPinned,
		RestrictedFor: post.RestrictedFor,
		CommentCount:  post.CommentCount,
		Assets:        []fanboxPostAssetOut{},
	}
	if post.Body == nil {
		return out
	}
	for _, asset := range post.Body.Assets {
		out.Assets = append(out.Assets, fanboxPostAssetOut{
			ID:        asset.ID,
			Kind:      string(asset.Kind),
			Name:      asset.Name,
			Resource:  fanboxResourceOutFrom(asset.Resource),
			Thumbnail: fanboxResourceOutFrom(asset.Thumbnail.Resource),
		})
	}
	return out
}

type fanboxPostsOut struct {
	Posts      []fanboxPostOut     `json:"posts"`
	Pagination fanboxPaginationOut `json:"pagination"`
}

// fanboxPostList 打开 client 后统一收集帖子分页并输出。
func (a *App) fanboxPostList(ctx context.Context, client *fanboxsdk.Client, limit *int, plan fanboxListPlan, fetch func(context.Context, *fanboxsdk.Client, sdk.Cursor) (sdk.Page[fanboxsdk.Post], error)) (*mcp.CallToolResult, fanboxPostsOut, error) {
	out := fanboxPostsOut{Posts: []fanboxPostOut{}}
	items, hasMore, err := collectFanboxPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return fetch(ctx, client, cursor)
	})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	for _, post := range items {
		out.Posts = append(out.Posts, fanboxPostOutFrom(post))
	}
	out.Pagination = fanboxListPagination(plan, limit, len(out.Posts), hasMore)
	return fanboxResult(out, false, fmt.Sprintf("Retrieved %d posts.", len(out.Posts))), out, nil
}

type creatorPostsIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
	fanboxListIn
}

func (a *App) creatorPosts(ctx context.Context, _ *mcp.CallToolRequest, in creatorPostsIn) (*mcp.CallToolResult, fanboxPostsOut, error) {
	out := fanboxPostsOut{Posts: []fanboxPostOut{}}
	plan, err := parseFanboxListPlan(in.fanboxListIn)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	if in.CreatorID == "" {
		return fanboxResult(out, true, "Error: creator_id is required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	creatorID := in.CreatorID
	return a.fanboxPostList(ctx, client, in.Limit, plan, func(ctx context.Context, client *fanboxsdk.Client, cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.CreatorPosts(ctx, fanboxsdk.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
	})
}

type taggedPostsIn struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
	Tag       string `json:"tag" jsonschema:"required tag name"`
	fanboxListIn
}

func (a *App) taggedPosts(ctx context.Context, _ *mcp.CallToolRequest, in taggedPostsIn) (*mcp.CallToolResult, fanboxPostsOut, error) {
	out := fanboxPostsOut{Posts: []fanboxPostOut{}}
	plan, err := parseFanboxListPlan(in.fanboxListIn)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	if in.CreatorID == "" || in.Tag == "" {
		return fanboxResult(out, true, "Error: creator_id and tag are required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	creatorID, tag := in.CreatorID, in.Tag
	return a.fanboxPostList(ctx, client, in.Limit, plan, func(ctx context.Context, client *fanboxsdk.Client, cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.TaggedPosts(ctx, fanboxsdk.TaggedPostsRequest{CreatorID: creatorID, Tag: tag, Cursor: cursor})
	})
}

type postIn struct {
	PostID string `json:"post_id" jsonschema:"required FANBOX post id"`
}

func (a *App) post(ctx context.Context, _ *mcp.CallToolRequest, in postIn) (*mcp.CallToolResult, fanboxPostOut, error) {
	out := fanboxPostOut{}
	if in.PostID == "" {
		return fanboxResult(out, true, "Error: post_id is required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	post, err := client.Post(ctx, fanboxsdk.PostRequest{PostID: in.PostID})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	out = fanboxPostOutFrom(post)
	return fanboxResult(out, false, fmt.Sprintf("Retrieved post %s.", out.ID)), out, nil
}

func (a *App) home(ctx context.Context, _ *mcp.CallToolRequest, in fanboxListIn) (*mcp.CallToolResult, fanboxPostsOut, error) {
	out := fanboxPostsOut{Posts: []fanboxPostOut{}}
	plan, err := parseFanboxListPlan(in)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	return a.fanboxPostList(ctx, client, in.Limit, plan, func(ctx context.Context, client *fanboxsdk.Client, cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.Home(ctx, fanboxsdk.HomeRequest{Cursor: cursor})
	})
}

func (a *App) supporting(ctx context.Context, _ *mcp.CallToolRequest, in fanboxListIn) (*mcp.CallToolResult, fanboxPostsOut, error) {
	out := fanboxPostsOut{Posts: []fanboxPostOut{}}
	plan, err := parseFanboxListPlan(in)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	return a.fanboxPostList(ctx, client, in.Limit, plan, func(ctx context.Context, client *fanboxsdk.Client, cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.Supporting(ctx, fanboxsdk.SupportingRequest{Cursor: cursor})
	})
}

// ---------------------------------------------------------------------------
// resolve URL / open resource
// ---------------------------------------------------------------------------

type resolveURLIn struct {
	URL string `json:"url" jsonschema:"required FANBOX page URL"`
}

type resolveURLOut struct {
	Kind      string `json:"kind"`
	CreatorID string `json:"creator_id,omitempty"`
	PostID    string `json:"post_id,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

func (a *App) resolveURL(ctx context.Context, _ *mcp.CallToolRequest, in resolveURLIn) (*mcp.CallToolResult, resolveURLOut, error) {
	out := resolveURLOut{}
	if strings.TrimSpace(in.URL) == "" {
		return fanboxResult(out, true, "Error: url is required"), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	ref, err := client.ResolveURL(ctx, fanboxsdk.ResolveURLRequest{RawURL: in.URL})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	out = resolveURLOut{Kind: string(ref.Kind), CreatorID: ref.CreatorID, PostID: ref.PostID, Tag: ref.Tag}
	return fanboxResult(out, false, fmt.Sprintf("Resolved URL as %s.", out.Kind)), out, nil
}

type openResourceIn struct {
	Ref    string `json:"ref" jsonschema:"opaque FANBOX media resource reference"`
	Method string `json:"method,omitempty" jsonschema:"GET or HEAD; default GET"`
}

type openResourceOut struct {
	Ref           string            `json:"ref"`
	StatusCode    int               `json:"status_code"`
	ContentType   string            `json:"content_type,omitempty"`
	ContentLength int64             `json:"content_length,omitempty"`
	Headers       map[string]string `json:"headers"`
}

func (a *App) openResource(ctx context.Context, _ *mcp.CallToolRequest, in openResourceIn) (*mcp.CallToolResult, openResourceOut, error) {
	out := openResourceOut{Ref: in.Ref, Headers: map[string]string{}}
	ref, err := sdk.ParseResourceRef(in.Ref)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	method := sdk.ResourceMethod(strings.ToUpper(in.Method))
	switch method {
	case "", sdk.ResourceMethodGet, sdk.ResourceMethodHead:
	default:
		return fanboxResult(out, true, `Error: method supports only "GET" or "HEAD"`), out, nil
	}
	client, err := a.openClient(ctx)
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: method})
	if err != nil {
		return fanboxResult(out, true, "Error: "+err.Error()), out, nil
	}
	// 只返回 URL 消费所需的 headers 与 status，绝不把字节返回给调用方。
	defer response.Body.Close()
	out.StatusCode = response.StatusCode
	out.ContentType = response.ContentType()
	out.ContentLength = response.ContentLength()
	out.Headers = fanboxHeaderMap(response.Header())
	return fanboxResult(out, false, fmt.Sprintf("Resource status %d.", response.StatusCode)), out, nil
}

func fanboxHeaderMap(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		if len(values) > 0 {
			out[name] = values[0]
		}
	}
	return out
}
