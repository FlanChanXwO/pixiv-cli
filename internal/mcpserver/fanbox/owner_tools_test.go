package fanbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 以下 9 个 FANBOX tool 的 owner 契约：请求映射（输入 → 上游参数）、成功输出
// 形状与一条错误分类路径。每个测试断言上游 URL 的 path 与 query，并核对
// structured 输出的关键字段。

func TestFanboxCreatorMapsCreatorIDAndReturnsProfile(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"creatorId":"creator value","user":{"name":"Creator Name","iconUrl":"https://i.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://i.pximg.net/cover.png","plan":{"fee":500,"hasSupportingPlan":true}}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator", Arguments: map[string]any{"creator_id": "creator value"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/creator.get?creatorId=creator+value" {
		t.Fatalf("creator.get URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_creator failed: %+v", result)
	}
	var out struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		HasAdultContent   bool   `json:"has_adult_content"`
		IsFollowing       bool   `json:"is_following"`
		PlanFee           int    `json:"plan_fee"`
		HasSupportingPlan bool   `json:"has_supporting_plan"`
		Icon              any    `json:"icon"`
	}
	decodeStructured(t, result, &out)
	if out.ID != "creator value" || out.Name != "Creator Name" || !out.HasAdultContent || !out.IsFollowing || out.PlanFee != 500 || !out.HasSupportingPlan || out.Icon == nil {
		t.Fatalf("creator output=%+v", out)
	}
}

func TestFanboxCreatorMissingIDIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator", Arguments: map[string]any{"creator_id": ""}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error: creator_id is required") {
		t.Fatalf("missing creator_id result=%+v", result)
	}
}

func TestFanboxCreatorPostsMapsCreatorIDAndReturnsPosts(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"posts":[{"id":"p1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":[]}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator_posts", Arguments: map[string]any{"creator_id": "c"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/post.listCreator?creatorId=c&limit=10" {
		t.Fatalf("post.listCreator URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_creator_posts failed: %+v", result)
	}
	var out struct {
		Posts []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"posts"`
	}
	decodeStructured(t, result, &out)
	if len(out.Posts) != 1 || out.Posts[0].ID != "p1" || out.Posts[0].Title != "a" {
		t.Fatalf("creator posts output=%+v", out)
	}
}

func TestFanboxCreatorPostsMissingIDIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator_posts", Arguments: map[string]any{"creator_id": ""}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error: creator_id is required") {
		t.Fatalf("missing creator_id result=%+v", result)
	}
}

func TestFanboxCreatorTagsMapsCreatorIDAndReturnsTags(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"tags":[{"tag":"fanart","url":"https://www.fanbox.cc/@writer/posts/tag/fanart"}]}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator_tags", Arguments: map[string]any{"creator_id": "writer"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/tag.getFeatured?creatorId=writer" {
		t.Fatalf("tag.getFeatured URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_creator_tags failed: %+v", result)
	}
	var out struct {
		Tags []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"tags"`
	}
	decodeStructured(t, result, &out)
	if len(out.Tags) != 1 || out.Tags[0].Name != "fanart" || out.Tags[0].URL != "https://www.fanbox.cc/@writer/posts/tag/fanart" {
		t.Fatalf("creator tags output=%+v", out)
	}
}

func TestFanboxCreatorTagsMissingIDIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creator_tags", Arguments: map[string]any{"creator_id": ""}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error: creator_id is required") {
		t.Fatalf("missing creator_id result=%+v", result)
	}
}

func TestFanboxCreatorsMapsKindAndReturnsSummaries(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"plans":[{"creatorId":"supported"}],"pageUrls":[]}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creators", Arguments: map[string]any{"kind": "supporting"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/plan.listSupporting?" {
		t.Fatalf("plan.listSupporting URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_creators failed: %+v", result)
	}
	var out struct {
		Creators []struct {
			ID string `json:"id"`
		} `json:"creators"`
	}
	decodeStructured(t, result, &out)
	if len(out.Creators) != 1 || out.Creators[0].ID != "supported" {
		t.Fatalf("creators output=%+v", out)
	}
}

func TestFanboxCreatorsInvalidKindIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_creators", Arguments: map[string]any{"kind": "everything"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error: kind must be one of: supporting, following") {
		t.Fatalf("invalid kind result=%+v", result)
	}
}

func TestFanboxHomeMapsFeedAndReturnsPosts(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"items":[{"id":"h1","title":"home","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"nextUrl":""}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_home", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/post.listHome?limit=10" {
		t.Fatalf("post.listHome URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_home failed: %+v", result)
	}
	var out struct {
		Posts []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"posts"`
	}
	decodeStructured(t, result, &out)
	if len(out.Posts) != 1 || out.Posts[0].ID != "h1" || out.Posts[0].Title != "home" {
		t.Fatalf("home output=%+v", out)
	}
}

func TestFanboxHomeUpstreamFailureIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("home upstream refused")
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_home", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error:") || result.StructuredContent == nil {
		t.Fatalf("home failure result=%+v", result)
	}
}

func TestFanboxPostMapsPostIDAndReturnsPost(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"post":{"id":"p1","title":"a","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false,"body":{"blocks":[{"type":"image","imageId":"image-1"}],"imageMap":{"image-1":{"id":"image-1","extension":"png","originalUrl":"https://downloads.fanbox.cc/image.png"}}}}}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_post", Arguments: map[string]any{"post_id": "p1"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/post.info?postId=p1" {
		t.Fatalf("post.info URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_post failed: %+v", result)
	}
	var out struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	decodeStructured(t, result, &out)
	if out.ID != "p1" || out.Title != "a" {
		t.Fatalf("post output=%+v", out)
	}
}

func TestFanboxPostMissingIDIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_post", Arguments: map[string]any{"post_id": ""}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error: post_id is required") {
		t.Fatalf("missing post_id result=%+v", result)
	}
}

func TestFanboxResolveURLParsesPageURL(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_resolve_url", Arguments: map[string]any{"url": "https://www.fanbox.cc/@writer/posts/123"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("fanbox_resolve_url failed: %+v", result)
	}
	var out struct {
		Kind   string `json:"kind"`
		PostID string `json:"post_id"`
	}
	decodeStructured(t, result, &out)
	if out.Kind != "post" || out.PostID != "123" {
		t.Fatalf("resolve output=%+v", out)
	}
}

func TestFanboxResolveURLInvalidHostIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_resolve_url", Arguments: map[string]any{"url": "https://example.com/not-fanbox"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error:") {
		t.Fatalf("invalid host result=%+v", result)
	}
}

func TestFanboxSupportingMapsFeedAndReturnsPosts(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"items":[{"id":"s1","title":"support","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"nextUrl":""}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_supporting", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/post.listSupporting?limit=10" {
		t.Fatalf("post.listSupporting URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_supporting failed: %+v", result)
	}
	var out struct {
		Posts []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"posts"`
	}
	decodeStructured(t, result, &out)
	if len(out.Posts) != 1 || out.Posts[0].ID != "s1" || out.Posts[0].Title != "support" {
		t.Fatalf("supporting output=%+v", out)
	}
}

func TestFanboxSupportingUpstreamFailureIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("supporting upstream refused")
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_supporting", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError || !strings.Contains(textOf(t, result), "Error:") || result.StructuredContent == nil {
		t.Fatalf("supporting failure result=%+v", result)
	}
}

func TestFanboxTaggedPostsMapsCreatorAndTag(t *testing.T) {
	var captured string
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.URL.Path + "?" + req.URL.RawQuery
		return jsonResponse(`{"body":{"posts":[{"id":"t1","title":"tag","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":[]}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_tagged_posts", Arguments: map[string]any{"creator_id": "c", "tag": "fanart"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if captured != "/post.listTagged?creatorId=c&tag=fanart" {
		t.Fatalf("post.listTagged URL = %q", captured)
	}
	if result.IsError {
		t.Fatalf("fanbox_tagged_posts failed: %+v", result)
	}
	var out struct {
		Posts []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"posts"`
	}
	decodeStructured(t, result, &out)
	if len(out.Posts) != 1 || out.Posts[0].ID != "t1" || out.Posts[0].Title != "tag" {
		t.Fatalf("tagged posts output=%+v", out)
	}
}

func TestFanboxTaggedPostsMissingCreatorOrTagIsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	for _, args := range []map[string]any{{"creator_id": "", "tag": ""}, {"creator_id": "c", "tag": ""}} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_tagged_posts", Arguments: args})
		if err != nil {
			t.Fatalf("call tool: %v", err)
		}
		if !result.IsError || !strings.Contains(textOf(t, result), "Error: creator_id and tag are required") {
			t.Fatalf("args=%v result=%+v", args, result)
		}
	}
}

func textOf(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("result content=%+v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("result content type=%T", result.Content[0])
	}
	return text.Text
}

// 运行期输出安全 canary：真实调用 fanbox_post，上游 post body 的图片 URL 携带
// 签名 canary query（host 通过 media policy），structured 与 text 输出都不能带它。
func TestFanboxPostOutputValuesDoNotLeakResourceCanary(t *testing.T) {
	const canaryURL = "https://downloads.fanbox.cc/post/1/image.png?signature=topsecret"
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"body":{"post":{"id":"p1","title":"t","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false,"body":{"blocks":[{"type":"image","imageId":"image-1"}],"imageMap":{"image-1":{"id":"image-1","extension":"png","originalUrl":"` + canaryURL + `"}}}}}}`), nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_post", Arguments: map[string]any{"post_id": "p1"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("fanbox_post failed: %+v", result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canaryURL) || strings.Contains(string(raw), "signature=topsecret") {
		t.Fatalf("fanbox_post structured output leaked resource canary: %s", raw)
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && strings.Contains(text.Text, canaryURL) {
			t.Fatalf("fanbox_post text output leaked resource canary: %s", text.Text)
		}
	}
}
