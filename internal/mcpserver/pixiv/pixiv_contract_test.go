package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKMutationTypedErrorIsMCPError(t *testing.T) {
	client := &fakeSDKClient{addBookmarkErr: &sdk.Error{
		Product:    "pixiv",
		Operation:  "AddBookmark",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 41})
	if !result.IsError {
		t.Fatalf("typed SDK mutation failure must be an MCP error: %+v", result)
	}
	var out outputs.Mutation
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured mutation error = %+v", out)
	}
}

func TestSDKMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name     string
		args     map[string]any
		want     string
		wantText string
	}{
		{"add_bookmark", map[string]any{"illust_id": 9, "restrict": "private", "tags": []string{"one"}}, "add_bookmark", "Bookmarked artwork 9."},
		{"remove_bookmark", map[string]any{"illust_id": 9}, "remove_bookmark", "Removed bookmark from artwork 9."},
		{"follow_user", map[string]any{"user_id": 8, "restrict": "private"}, "follow_user", "Followed user 8."},
		{"unfollow_user", map[string]any{"user_id": 8}, "unfollow_user", "Unfollowed user 8."},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			var out outputs.Mutation
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.want || out.Text != test.wantText {
				t.Fatalf("mutation output = %+v", out)
			}
		})
	}
	if client.addBookmarkRequest.ArtworkID != 9 || client.addBookmarkRequest.Restrict != pixiv.RestrictPrivate || !slices.Equal(client.addBookmarkRequest.Tags, []string{"one"}) {
		t.Fatalf("add bookmark request = %+v", client.addBookmarkRequest)
	}
	if client.removeBookmarkRequest.ArtworkID != 9 || client.followUserRequest.UserID != 8 || client.followUserRequest.Restrict != pixiv.RestrictPrivate || client.unfollowUserRequest.UserID != 8 {
		t.Fatalf("mutation requests = remove=%+v follow=%+v unfollow=%+v", client.removeBookmarkRequest, client.followUserRequest, client.unfollowUserRequest)
	}
}

// unsafeOutputPropertyNames are JSON property names that would expose request
// transport or credential material to an MCP client. A tool output may carry
// an opaque resource reference, never the headers, cookies, signed locator or
// expiry needed to replay a request outside the SDK.
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
	"token":           {},
}

// TestEveryToolOutputSchemaOmitsTransportAndCredentialFields walks the output
// schema of every registered Pixiv tool as an MCP client sees it and rejects
// any property that would hand a client replayable transport state.
//
// This asserts against the live registry rather than source text, so adding a
// tool, renaming its output type, or moving it to another package cannot slip
// past the check. A tool without an output schema is reported too: an
// unschematized output is exactly the case where an unsafe field would go
// unnoticed.
func TestEveryToolOutputSchemaOmitsTransportAndCredentialFields(t *testing.T) {
	tools := connectAndListTools(t)
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	violations := unsafeSchemaProperties(t, tools)
	if len(violations) > 0 {
		t.Fatalf("tool output schemas expose transport or credential fields:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestUnsafeOutputPropertyIsDetected proves the walk above can fail. Without
// it, a schema shape the walker does not understand would silently turn the
// check into a no-op for every tool.
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
// keeps the check independent of which schema keywords the generator emits, so
// a nested `$defs`, `items`, `anyOf` or vendor extension cannot hide a field.
// JSON Schema keywords themselves are never in the forbidden set, so this
// cannot produce a false positive from schema structure alone.
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

func connectAndListTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	server := pixivmcpserver.New(&fakeAPI{}, &fakeDownloads{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools := make([]*mcp.Tool, 0)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

// TestArtworkDTOOptionalFieldsAreNotRequiredInSchemas 防止 DTO 的 omitempty 修复
// 被回退：`updated_at`/`tools`/`pages` 无论嵌套在输出 schema 的哪一层（例如
// `trending_tags_illust` 的 `tags.items.artwork`），只要出现就必须是可选的
// （不在同层 required 列表），否则真实输出会在字段缺失时违反自己声明的 schema。
func TestArtworkDTOOptionalFieldsAreNotRequiredInSchemas(t *testing.T) {
	tools := connectAndListTools(t)
	optional := map[string]bool{"updated_at": false, "tools": false, "pages": false}
	for _, tool := range tools {
		raw, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			t.Fatal(err)
		}
		assertOptionalSchemaKeys(t, tool.Name, node, optional)
	}
	for key, seen := range optional {
		if !seen {
			t.Fatalf("optional DTO field %q never appears in any tool output schema", key)
		}
	}
}

// assertOptionalSchemaKeys 递归遍历 schema 节点：遇到 object 节点时，对 properties
// 中出现的 optional key 断言其不在同层 required 列表，并继续下钻。
func assertOptionalSchemaKeys(t *testing.T, toolName string, node any, optional map[string]bool) {
	t.Helper()
	switch value := node.(type) {
	case map[string]any:
		properties, _ := value["properties"].(map[string]any)
		required := make(map[string]bool)
		if rawRequired, ok := value["required"].([]any); ok {
			for _, name := range rawRequired {
				if s, ok := name.(string); ok {
					required[s] = true
				}
			}
		}
		for key := range optional {
			if _, ok := properties[key]; ok {
				optional[key] = true
				if required[key] {
					t.Fatalf("tool %q schema requires optional DTO field %q", toolName, key)
				}
			}
		}
		for _, child := range value {
			assertOptionalSchemaKeys(t, toolName, child, optional)
		}
	case []any:
		for _, child := range value {
			assertOptionalSchemaKeys(t, toolName, child, optional)
		}
	}
}

const (
	// canaryURL 必须通过 SDK 的 resource URL policy（https + i.pximg.net host），
	// 同时在 query 里携带 signature canary；handler 若把原始 URL 复制进输出，
	// 就一定会带上 query。
	canaryURL    = "https://i.pximg.net/artworks/101/1.png?signature=topsecret"
	canaryCookie = "canary-session=topsecret"
	canaryAuth   = "Bearer canary-token"
)

func canaryStrings() []string {
	return []string{canaryURL, canaryCookie, canaryAuth, "topsecret"}
}

func resourceWithCanary() sdk.Resource {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"kind":"artwork-cover","id":101}`))
	if err != nil {
		panic(err)
	}
	return sdk.Resource{
		Ref:            ref,
		URL:            canaryURL,
		RequestHeaders: map[string]string{"Cookie": canaryCookie, "Authorization": canaryAuth},
	}
}

func assertNoCanaryLeak(t *testing.T, tool string, result *mcp.CallToolResult) {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	joined := strings.ToLower(string(raw))
	for _, c := range canaryStrings() {
		if strings.Contains(joined, strings.ToLower(c)) {
			t.Fatalf("tool %s structured output leaked canary %q: %s", tool, c, raw)
		}
	}
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		lower := strings.ToLower(text.Text)
		for _, c := range canaryStrings() {
			if strings.Contains(lower, strings.ToLower(c)) {
				t.Fatalf("tool %s text output leaked canary %q: %s", tool, c, text.Text)
			}
		}
	}
}

// TestToolOutputValuesDoNotLeakResourceTransportCanary 覆盖资源/实体读取路径：
// fake SDK 返回的 artwork/bookmark cover 携带签名 URL 与 Cookie，handler 必须只
// 输出 opaque resource ref，不能把 URL/Cookie 复制进结构化输出或文本摘要。
func TestToolOutputValuesDoNotLeakResourceTransportCanary(t *testing.T) {
	client := &fakeSDKClient{
		userID: 7,
		artworks: []pixiv.Artwork{{
			ID:    101,
			Title: "canary-art",
			Kind:  pixiv.ArtworkKindIllustration,
			User:  pixiv.User{ID: 7, Name: "artist"},
			Cover: pixiv.ImageResource{Resource: resourceWithCanary()},
		}},
		bookmarks: []pixiv.Artwork{{
			ID:    102,
			Title: "canary-bookmark",
			Kind:  pixiv.ArtworkKindIllustration,
			User:  pixiv.User{ID: 7, Name: "artist"},
			Cover: pixiv.ImageResource{Resource: resourceWithCanary()},
		}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"user_artworks", map[string]any{"user_id": 7, "limit": 1}},
		{"user_bookmarks", map[string]any{"user_id": 7, "limit": 1}},
	} {
		result := callTool(t, session, tc.tool, tc.args)
		if result.IsError {
			t.Fatalf("tool %s failed: %+v", tc.tool, result)
		}
		assertNoCanaryLeak(t, tc.tool, result)
	}
}

// TestToolErrorOutputDoesNotLeakCanary 覆盖错误路径：SDK 返回带 canary 的上游
// 错误文本时，MCP error result 的 text 与 structured 输出都不能原样带上它。
func TestToolErrorOutputDoesNotLeakCanary(t *testing.T) {
	client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{}, fmt.Errorf("upstream refused %s %s", canaryURL, canaryAuth)
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "canary"})
	if !result.IsError {
		t.Fatalf("search must fail: %+v", result)
	}
	assertNoCanaryLeak(t, "search_illust(error)", result)
}

// illust_ranking 的 owner 契约：请求映射与全部 mode。
