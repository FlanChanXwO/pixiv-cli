package fanbox_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	ugoira "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

// TestFanboxReadToolsReturnDeclaredEnvelopes 是 FANBOX 读取族的正常用例：
// 每个 tool 返回其声明的 structured 顶层字段；字段映射属于 sdk/fanbox 契约。
func TestFanboxReadToolsReturnDeclaredEnvelopes(t *testing.T) {
	for _, test := range []struct {
		name  string
		tool  string
		args  map[string]any
		body  string
		shape []string
	}{
		{name: "creator", tool: "fanbox_creator", args: map[string]any{"creator_id": "c"},
			body:  `{"body":{"creatorId":"c","user":{"name":"n","iconUrl":"https://i.pximg.net/i.png"},"coverImageUrl":"https://i.pximg.net/c.png","plan":{"fee":500,"hasSupportingPlan":true}}}`,
			shape: []string{"cover", "has_supporting_plan", "icon", "id", "name", "plan_fee"}},
		{name: "creators", tool: "fanbox_creators", args: map[string]any{"kind": "supporting"},
			body:  `{"body":{"plans":[{"creatorId":"c"}],"pageUrls":[]}}`,
			shape: []string{"creators", "pagination"}},
		{name: "creator tags", tool: "fanbox_creator_tags", args: map[string]any{"creator_id": "c"},
			body:  `{"body":{"tags":[{"tag":"t","url":"https://www.fanbox.cc/@w/posts/tag/t"}]}}`,
			shape: []string{"tags"}},
		{name: "post", tool: "fanbox_post", args: map[string]any{"post_id": "p1"},
			body:  `{"body":{"post":{"id":"p1","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"c","body":{"blocks":[],"imageMap":{}}}}}`,
			shape: []string{"assets", "creator_id", "id", "is_restricted", "published_at", "title"}},
		{name: "resolve url", tool: "fanbox_resolve_url", args: map[string]any{"url": "https://www.fanbox.cc/@w/posts/123"},
			shape: []string{"creator_id", "kind", "post_id"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _ := fanboxTestService(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(test.body), nil
			}))
			session, closeSession := newFanboxMCPSession(t, service)
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.args})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if result.IsError {
				t.Fatalf("%s failed: %+v", test.tool, result)
			}
			var structured map[string]any
			decodeStructured(t, result, &structured)
			keys := make([]string, 0, len(structured))
			for key := range structured {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if !slices.Equal(keys, test.shape) {
				t.Fatalf("%s structured keys=%v, want %v", test.tool, keys, test.shape)
			}
		})
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
	// 服务现在按已保存的默认账号打开 client；fixture 先落一个账号再注入 transport。
	stored := accountfanbox.New(42, "fixture", "", []byte("stored-session"))
	stored.ValidatedAt = 1
	if err := db.SaveFanboxCredential(t.Context(), stored); err != nil {
		t.Fatalf("save fanbox account: %v", err)
	}
	service := fanboxapp.NewService(fanboxMCPRepository{db: db}, fanboxMCPDefaults{}, func() (fanboxsdk.Options, error) {
		return fanboxsdk.Options{HTTPClient: &http.Client{Transport: rt}}, nil
	})
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
	return config.DefaultStore().ReadFanboxDefaultUserID()
}

func (fanboxMCPDefaults) SetFanboxDefaultUserID(userID int64) error {
	return config.DefaultStore().SetFanboxDefaultUserID(userID)
}

func (fanboxMCPDefaults) ClearFanboxDefaultUserID() error {
	return config.DefaultStore().ClearFanboxDefaultUserID()
}

func newFanboxMCPSession(t *testing.T, service *fanboxapp.Service) (*mcp.ClientSession, func()) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	fanboxmcpserver.Register(server, fanboxmcpserver.SDKPorts{
		Open: func(ctx context.Context, account fanboxmcpserver.Account) (*fanboxsdk.Client, error) {
			return service.OpenClient(ctx, account.HTTPSProxyOverride)
		},
	}, nil)
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
	var out struct {
		UserID      int64  `json:"user_id"`
		DisplayName string `json:"display_name"`
	}
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
	var out struct {
		UserID      int64  `json:"user_id"`
		DisplayName string `json:"display_name"`
	}
	decodeStructured(t, result, &out)
	if out.UserID != 0 {
		t.Fatalf("structured error output=%+v", out)
	}
}

func TestFanboxMCPOpenResourceDeliversImagesAndFiles(t *testing.T) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "frames.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archive)
	frame, err := zipWriter.Create("frame.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := frame.Write(pngBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(zipWriter.Close(), archive.Close()); err != nil {
		t.Fatal(err)
	}
	animationPath := filepath.Join(dir, "animation.apng")
	if err := ugoira.NewEncoder().Encode(t.Context(), ugoira.Input{ZipPath: archivePath, Frames: []ugoira.Frame{{File: "frame.png", Delay: 80}}, OutputPath: animationPath, Format: ugoira.FormatAPNG}); err != nil {
		t.Fatal(err)
	}
	apngData, err := os.ReadFile(animationPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, mime, method string
		header             string
		image              bool
		data               []byte
		status             int
	}{
		{name: "image", mime: "image/png", method: "GET", data: pngBuf.Bytes(), status: 200, image: true},
		{name: "file", mime: "application/pdf", method: "GET", data: []byte("%PDF-1.7\nfixture"), status: 200},
		{name: "filename", mime: "application/pdf", method: "GET", data: []byte("%PDF-1.7\nfixture"), status: 200},
		{name: "head", mime: "application/pdf", method: "HEAD", status: 200},
		{name: "forbidden", mime: "text/html", method: "GET", data: []byte("fixture-private-upstream-body"), status: 403},
		{name: "apng", mime: "image/apng", header: "image/png", method: "GET", data: apngData, status: 200},
		{name: "false-apng-header", mime: "image/png", header: "image/apng", method: "GET", data: pngBuf.Bytes(), status: 200, image: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			postBody := `{"body":{"post":{"id":"p-open","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://i.pximg.net/image-1.png"}]}}}}`
			header := tc.header
			if header == "" {
				header = tc.mime
			}
			host := "i.pximg.net"
			if tc.name == "file" || tc.name == "head" || tc.name == "filename" {
				host = "downloads.fanbox.cc"
				postBody = `{"body":{"post":{"id":"p-open","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"files":[{"id":"file-1","name":"fixture","extension":"pdf","url":"https://downloads.fanbox.cc/file-1.pdf"}]}}}}`
			}
			if tc.name == "filename" {
				postBody = strings.Replace(postBody, `"name":"fixture"`, `"name":"folder/报告.PDF"`, 1)
			}
			service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
					return jsonResponse(postBody), nil
				}
				if req.URL.Host != host {
					t.Errorf("unexpected fixture request: %s", req.URL.Host)
					return nil, errors.New("unexpected request")
				}
				if req.Method != tc.method {
					t.Errorf("method=%s want %s", req.Method, tc.method)
				}
				if (req.Header.Get("Cookie") != "") != (host == "downloads.fanbox.cc") {
					t.Error("credentialed resource host policy mismatch")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {header}, "Content-Length": {strconv.Itoa(len(tc.data))}}, Body: io.NopCloser(bytes.NewReader(tc.data))}, nil
			}))
			session, closeSession := newFanboxMCPSession(t, service)
			defer closeSession()
			client, err := service.OpenClient(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			post, err := client.Post(t.Context(), fanboxsdk.PostRequest{PostID: "p-open"})
			if err != nil {
				t.Fatal(err)
			}
			ref := post.Body.Assets[0].Resource.Ref
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fanbox_open_resource", Arguments: map[string]any{"ref": ref.String(), "method": tc.method}})
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Filename      string `json:"filename,omitempty"`
				Ref           string `json:"ref"`
				StatusCode    int    `json:"status_code"`
				ContentType   string `json:"content_type,omitempty"`
				ContentLength int64  `json:"content_length,omitempty"`
				Size          int    `json:"size"`
				ContentIndex  *int   `json:"content_index,omitempty"`
				Delivered     bool   `json:"delivered"`
				Complete      bool   `json:"complete"`
				Error         string `json:"error,omitempty"`
			}
			decodeStructured(t, result, &out)
			if tc.status == 200 && out.StatusCode != tc.status {
				t.Fatalf("status=%d", out.StatusCode)
			}
			if tc.status != 200 {
				if !result.IsError || out.Error != "forbidden" {
					t.Fatal("HTTP failure reported success")
				}
				raw, _ := json.Marshal(result)
				if bytes.Contains(raw, tc.data) {
					t.Fatal("raw upstream body leaked")
				}
				return
			}
			if result.IsError {
				t.Fatalf("resource failed: %+v", result)
			}
			if out.ContentType != tc.mime {
				t.Fatalf("MIME=%q", out.ContentType)
			}
			if tc.name == "file" || tc.name == "head" || tc.name == "filename" {
				var named struct {
					Filename string `json:"filename"`
				}
				decodeStructured(t, result, &named)
				wantFilename := "fixture.pdf"
				if tc.name == "filename" {
					wantFilename = "folder_报告.PDF"
				}
				if named.Filename != wantFilename {
					t.Fatalf("resource filename=%q, want %q", named.Filename, wantFilename)
				}
			}
			if tc.method == "HEAD" {
				if out.Delivered || out.ContentIndex != nil {
					t.Fatal("HEAD claimed delivery")
				}
				for _, c := range result.Content {
					if _, ok := c.(*mcp.TextContent); !ok {
						t.Fatal("HEAD returned media bytes")
					}
				}
				return
			}
			if !out.Delivered || !out.Complete || out.Size != len(tc.data) || out.ContentIndex == nil || *out.ContentIndex != 0 {
				t.Fatalf("delivery metadata=%+v", out)
			}
			if tc.image {
				img, ok := result.Content[0].(*mcp.ImageContent)
				if !ok || !bytes.Equal(img.Data, tc.data) || img.MIMEType != tc.mime {
					t.Fatalf("missing real ImageContent: %#v", result.Content)
				}
			} else {
				blob, ok := result.Content[0].(*mcp.EmbeddedResource)
				if !ok || !bytes.Equal(blob.Resource.Blob, tc.data) || blob.Resource.MIMEType != tc.mime {
					t.Fatalf("missing real blob: %#v", result.Content)
				}
			}
		})
	}
}

func TestFanboxMCPOpenResourceRejectsInvalidRef(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()
	for _, args := range []map[string]any{{"ref": "fixture-private-invalid-ref"}, {"ref": []string{"fixture-private-schema"}}, {"ref": "invalid", "extra": "fixture-private-extra"}} {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fanbox_open_resource", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError || result.StructuredContent == nil {
			t.Fatalf("invalid input missing structured error: %+v", result)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "fixture-private") {
			t.Fatalf("input echoed: %s", raw)
		}
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

type fanboxWaitingBody struct {
	ctx             context.Context
	started, closed chan struct{}
}

func (b *fanboxWaitingBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *fanboxWaitingBody) Close() error { close(b.closed); return nil }

func TestFanboxMCPOpenResourceCancellationClosesBody(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	service, _ := fanboxTestService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.fanbox.cc" {
			return jsonResponse(`{"body":{"post":{"id":"p-cancel","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://i.pximg.net/image-1.png"}]}}}}`), nil
		}
		if r.URL.Host != "i.pximg.net" {
			return nil, errors.New("unexpected fixture host")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: &fanboxWaitingBody{ctx: r.Context(), started: started, closed: closed}}, nil
	}))
	client, err := service.OpenClient(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	post, err := client.Post(t.Context(), fanboxsdk.PostRequest{PostID: "p-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "fanbox_open_resource", Arguments: map[string]any{"ref": post.Body.Assets[0].Resource.Ref.String()}})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("resource body read did not start")
	}
	cancel()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not close resource body")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled call did not return")
	}
}

func TestConsolidatedFeedInputsRejectBeforeOpeningSession(t *testing.T) {
	opens := 0
	server := mcp.NewServer(&mcp.Implementation{Name: "input-validation", Version: "1"}, nil)
	fanboxmcpserver.Register(server, fanboxmcpserver.SDKPorts{Open: func(context.Context, fanboxmcpserver.Account) (*fanboxsdk.Client, error) {
		opens++
		return nil, errors.New("unexpected session access")
	}}, nil)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "validation-client", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"fanbox_feed", map[string]any{}},
		{"fanbox_feed", map[string]any{"source": "all"}},
		{"fanbox_feed", map[string]any{"source": "home", "unknown": true}},
		{"fanbox_feed", map[string]any{"source": "supporting", "page": 2}},
		{"fanbox_creator_posts", map[string]any{"creator_id": "c", "tag": ""}},
		{"fanbox_creator_posts", map[string]any{"creator_id": "c", "tag": nil}},
	} {
		result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("invalid %s args accepted: %+v", tc.name, result)
		}
		if opens != 0 {
			t.Fatalf("invalid %s args opened session %d times", tc.name, opens)
		}
	}
}
