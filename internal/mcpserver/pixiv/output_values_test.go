package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 运行期输出安全 canary 测试：不是检查 schema key，而是**实际调用 tool**，让
// SDK 返回值里携带 canary（签名 URL、Cookie、Authorization），再递归检查
// structured 输出与 text 输出的**值**不会泄漏它们。
//
// schema 层检查（TestEveryToolOutputSchemaOmitsTransportAndCredentialFields）
// 只能证明「没有 transport/credential 字段名」；handler 把 canary 塞进一个名字
// 无害的字段（例如 url 或 text）时 schema 不会变红。本测试补上值层面。

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
