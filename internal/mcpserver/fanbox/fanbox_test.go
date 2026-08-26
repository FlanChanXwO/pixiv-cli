package fanbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/currentuser"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/openresource"
	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestFanboxMCPDiagnosticsUseFanboxModuleAndRequestID(t *testing.T) {
	var (
		mu     sync.Mutex
		events []diagnostics.Event
	)
	sink := diagnostics.SinkFunc(func(event diagnostics.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	rootCtx, cancel := context.WithCancel(diagnostics.WithScope(context.Background(), sink, diagnostics.ModuleFanboxCLI, 0))
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "debug-test", Version: "1"}, nil)
	runtime.AddTool(runtime.NewApp(runtime.SDKPorts{}, runtime.Account{}), server, &mcp.Tool{Name: "diagnostic_test"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{}, struct{}{}, nil
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(rootCtx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "debug-client", Version: "1"}, nil)
	session, err := client.Connect(rootCtx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "diagnostic_test", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %+v", result)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("events=%+v want start/complete pair", events)
	}
	for _, event := range events {
		if event.Module != diagnostics.ModuleFanboxMCPServer || event.RequestID != 1 {
			t.Fatalf("event=%+v", event)
		}
	}
}

// unsafeOutputPropertyNames mirrors the Pixiv product contract: a FANBOX tool
// output may carry an opaque resource reference, never the session cookie,
// request headers, signed locator or expiry needed to replay a request outside
// the SDK.
//
// The list is intentionally duplicated per product rather than shared. Each
// MCP product owns its own output contract and its own registry construction,
// and a shared test package would couple the two products' test builds for
// twenty lines of table. If a third product appears, converge them then.
var unsafeOutputPropertyNames = map[string]struct{}{
	"cookie":          {},
	"cookies":         {},
	"expiry":          {},
	"expires_at":      {},
	"headers":         {},
	"locator":         {},
	"request_headers": {},
	"refresh_token":   {},
	"access_token":    {},
	"session":         {},
	"fanboxsessid":    {},
	"token":           {},
}

// TestEveryToolOutputSchemaOmitsTransportAndCredentialFields walks the output
// schema of every registered FANBOX tool as an MCP client sees it and rejects
// any property that would hand a client replayable transport or session state.
//
// FANBOX authenticates with a long-lived session cookie, so leaking transport
// metadata here is strictly worse than on the Pixiv side: a signed locator plus
// its headers is enough to fetch paid content outside the SDK.
func TestEveryToolOutputSchemaOmitsTransportAndCredentialFields(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	tools := listFanboxTools(t, session)
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	violations := unsafeSchemaProperties(t, tools)
	if len(violations) > 0 {
		t.Fatalf("tool output schemas expose transport or credential fields:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestUnsafeOutputPropertyIsDetected proves the walk above can fail, so a
// schema shape the walker does not understand cannot silently turn the check
// into a no-op for every tool.
func TestUnsafeOutputPropertyIsDetected(t *testing.T) {
	probe := []*mcp.Tool{{
		Name: "probe_tool",
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assets": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"cookie": map[string]any{"type": "string"}},
					},
				},
			},
		},
	}}

	violations := unsafeSchemaProperties(t, probe)
	if len(violations) != 1 || !strings.Contains(violations[0], "cookie") {
		t.Fatalf("nested unsafe property was not detected, got %v", violations)
	}
}

func listFanboxTools(t *testing.T, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	tools := make([]*mcp.Tool, 0)
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func unsafeSchemaProperties(t *testing.T, tools []*mcp.Tool) []string {
	t.Helper()
	violations := make([]string, 0)
	for _, tool := range tools {
		if tool.OutputSchema == nil {
			violations = append(violations, tool.Name+": no output schema")
			continue
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", tool.Name, err)
		}
		var schema any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s output schema: %v", tool.Name, err)
		}
		for _, path := range unsafeSchemaKeys(schema, "") {
			violations = append(violations, tool.Name+": "+path)
		}
	}
	sort.Strings(violations)
	return violations
}

// unsafeSchemaKeys reports every object key in the decoded schema whose name is
// forbidden, at any depth. Walking raw keys rather than only `properties` maps
// keeps the check independent of which schema keywords the generator emits.
func unsafeSchemaKeys(node any, path string) []string {
	found := make([]string, 0)
	switch value := node.(type) {
	case map[string]any:
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child := path + "/" + name
			if _, unsafe := unsafeOutputPropertyNames[strings.ToLower(name)]; unsafe {
				found = append(found, child)
			}
			found = append(found, unsafeSchemaKeys(value[name], child)...)
		}
	case []any:
		for index, child := range value {
			found = append(found, unsafeSchemaKeys(child, path+"/"+strconv.Itoa(index))...)
		}
	}
	return found
}

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
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{name: "creator", tool: "fanbox_creator", args: map[string]any{"creator_id": ""}, want: "creator_id is required"},
		{name: "creator posts", tool: "fanbox_creator_posts", args: map[string]any{"creator_id": ""}, want: "creator_id is required"},
		{name: "creator tags", tool: "fanbox_creator_tags", args: map[string]any{"creator_id": ""}, want: "creator_id is required"},
		{name: "post", tool: "fanbox_post", args: map[string]any{"post_id": ""}, want: "post_id is required"},
		{name: "tagged posts", tool: "fanbox_tagged_posts", args: map[string]any{"creator_id": "c", "tag": ""}, want: "creator_id and tag are required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
			session, closeSession := newFanboxMCPSession(t, service)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.args})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if !result.IsError || !strings.Contains(textOf(t, result), test.want) {
				t.Fatalf("missing argument result=%+v", result)
			}
		})
	}
}

func TestFanboxPostListToolsMapIDsAndReturnPosts(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		args      map[string]any
		wantURL   string
		body      string
		wantID    string
		wantTitle string
	}{
		{
			name: "creator posts", tool: "fanbox_creator_posts",
			args: map[string]any{"creator_id": "c"}, wantURL: "/post.listCreator?creatorId=c&limit=10",
			body:   `{"body":{"posts":[{"id":"p1","title":"creator posts","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":[]}}`,
			wantID: "p1", wantTitle: "creator posts",
		},
		{
			name: "home", tool: "fanbox_home", args: map[string]any{}, wantURL: "/post.listHome?limit=10",
			body:   `{"body":{"items":[{"id":"h1","title":"home","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"nextUrl":""}}`,
			wantID: "h1", wantTitle: "home",
		},
		{
			name: "supporting", tool: "fanbox_supporting", args: map[string]any{}, wantURL: "/post.listSupporting?limit=10",
			body:   `{"body":{"items":[{"id":"s1","title":"supporting","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"nextUrl":""}}`,
			wantID: "s1", wantTitle: "supporting",
		},
		{
			name: "tagged posts", tool: "fanbox_tagged_posts",
			args: map[string]any{"creator_id": "c", "tag": "fanart"}, wantURL: "/post.listTagged?creatorId=c&tag=fanart",
			body:   `{"body":{"posts":[{"id":"t1","title":"tagged","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":[]}}`,
			wantID: "t1", wantTitle: "tagged",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured string
			service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				captured = req.URL.Path + "?" + req.URL.RawQuery
				return jsonResponse(test.body), nil
			}))
			session, closeSession := newFanboxMCPSession(t, service)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.args})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if captured != test.wantURL {
				t.Fatalf("request URL = %q, want %q", captured, test.wantURL)
			}
			if result.IsError {
				t.Fatalf("%s failed: %+v", test.tool, result)
			}
			var out struct {
				Posts []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"posts"`
			}
			decodeStructured(t, result, &out)
			if len(out.Posts) != 1 || out.Posts[0].ID != test.wantID || out.Posts[0].Title != test.wantTitle {
				t.Fatalf("post output=%+v", out)
			}
		})
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
	for _, test := range []struct {
		name string
		tool string
	}{
		{name: "home", tool: "fanbox_home"},
		{name: "supporting", tool: "fanbox_supporting"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _ := fanboxTestService(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("upstream refused")
			}))
			session, closeSession := newFanboxMCPSession(t, service)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: map[string]any{}})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if !result.IsError || result.StructuredContent == nil || !strings.Contains(textOf(t, result), "Error:") {
				t.Fatalf("%s failure result=%+v", test.tool, result)
			}
		})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func homeMetadataBody(userID int64, name string) string {
	return `<html><head><meta name="metadata" content='{"context":{"user":{"userId":` + fmt.Sprintf("%d", userID) + `,"name":"` + name + `"}}}'></head></html>`
}

// fanboxTestService 构造注入 httptest transport 的 FANBOX 服务，绝不拨号网络。
func fanboxTestService(t *testing.T, rt http.RoundTripper) (*fanboxapp.Service, *database.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(paths.SetConfigFilePathForTest(filepath.Join(home, paths.AppDataDirName, "config.toml")))
	appDataDir := filepath.Join(home, paths.AppDataDirName)
	db, err := database.Open(appDataDir)
	if err != nil {
		t.Fatalf("open auth db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := fanboxapp.NewService(fanboxMCPRepository{db: db}, fanboxMCPDefaults{})
	service.OpenClientFunc = func(context.Context) (*fanboxsdk.Client, error) {
		return fanboxsdk.OpenWith(fanboxsdk.SessionCredentials{FANBOXSESSID: "stored-session"}, fanboxsdk.Options{HTTPClient: &http.Client{Transport: rt}})
	}
	return service, db
}

type fanboxMCPRepository struct{ db *database.DB }

func (r fanboxMCPRepository) SaveFanboxCredential(ctx context.Context, account accountfanbox.Account) error {
	return r.db.SaveFanboxCredential(ctx, account)
}

func (r fanboxMCPRepository) RotateFanboxSession(ctx context.Context, userID, revision int64, session []byte, validatedAt int64) error {
	return r.db.RotateFanboxSession(ctx, userID, revision, session, validatedAt)
}

func (r fanboxMCPRepository) ListFanbox(ctx context.Context) ([]accountfanbox.Account, error) {
	return r.db.ListFanbox(ctx)
}

func (r fanboxMCPRepository) GetFanbox(ctx context.Context, userID int64) (accountfanbox.Account, error) {
	return r.db.GetFanbox(ctx, userID)
}

func (r fanboxMCPRepository) RemoveFanbox(ctx context.Context, userID int64) error {
	return r.db.RemoveFanbox(ctx, userID)
}

type fanboxMCPDefaults struct{}

func (fanboxMCPDefaults) ReadFanboxDefaultUserID() (int64, bool, error) {
	return config.ReadFanboxDefaultUserID()
}

func (fanboxMCPDefaults) SetFanboxDefaultUserID(userID int64) error {
	return config.SetFanboxDefaultUserID(userID)
}

func (fanboxMCPDefaults) ClearFanboxDefaultUserID() error {
	return config.ClearFanboxDefaultUserID()
}

func newFanboxMCPSession(t *testing.T, service *fanboxapp.Service) (*mcp.ClientSession, func()) {
	t.Helper()
	server := fanboxmcpserver.New(fanboxmcpserver.SDKPorts{
		Open: func(ctx context.Context, account fanboxmcpserver.Account) (*fanboxsdk.Client, error) {
			return service.OpenClientWithProxy(ctx, account.HTTPSProxyOverride)
		},
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect: %v", err)
	}
	return session, func() {
		session.Close()
		cancel()
	}
}

func TestFanboxMCPCurrentUserHappyPath(t *testing.T) {
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_current_user", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("current_user failed: %+v", result)
	}
	var out currentuser.Out
	decodeStructured(t, result, &out)
	if out.UserID != 42 || out.DisplayName != "tester" {
		t.Fatalf("current_user output=%+v", out)
	}
}

func TestFanboxMCPFailureReturnsStructuredError(t *testing.T) {
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream refused")
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fanbox_current_user", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP error result: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("error result must retain text content: %+v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Error:") {
		t.Fatalf("error content=%#v", result.Content)
	}
	if result.StructuredContent == nil {
		t.Fatalf("structured content must be preserved on error")
	}
	var out currentuser.Out
	decodeStructured(t, result, &out)
	if out.UserID != 0 {
		t.Fatalf("structured error output=%+v", out)
	}
}

func TestFanboxMCPOpenResourceReturnsSafeMetadataWithoutBytes(t *testing.T) {
	postBody := `{"body":{"post":{"id":"p-open","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://i.pximg.net/image-1.png"}]}}}}`
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
			return jsonResponse(postBody), nil
		}
		if req.URL.Host == "i.pximg.net" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}, "Content-Length": {"4"}}, Body: io.NopCloser(strings.NewReader("PNG!"))}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	// Obtain a ref through the real SDK path so it carries only stable identity
	// (no embedded URL) and is bound to the in-session locator cache.
	client, err := service.OpenClient(context.Background())
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	post, err := client.Post(context.Background(), fanboxsdk.PostRequest{PostID: "p-open"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(post.Body.Assets) != 1 || post.Body.Assets[0].Resource.Ref.IsZero() {
		t.Fatalf("post resource = %+v", post.Body)
	}
	ref := post.Body.Assets[0].Resource.Ref

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "fanbox_open_resource",
		Arguments: map[string]any{"ref": ref.String()},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("open_resource failed: %+v", result)
	}
	var out openresource.Out
	decodeStructured(t, result, &out)
	if out.StatusCode != http.StatusOK || out.ContentType != "image/png" {
		t.Fatalf("open_resource output=%+v", out)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); ok && strings.Contains(text.Text, "PNG!") {
		t.Fatalf("resource bytes leaked into content: %s", text.Text)
	}
}

func TestFanboxMCPOpenResourceRejectsInvalidRef(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "fanbox_open_resource",
		Arguments: map[string]any{"ref": "not-a-valid-ref"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid ref must be an MCP error: %+v", result)
	}
}

func fanboxsdkOKRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"body":{}}`), nil
	})
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	if result.StructuredContent == nil {
		t.Fatalf("no structured content: %+v", result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode structured content: %v (raw=%s)", err, raw)
	}
}
