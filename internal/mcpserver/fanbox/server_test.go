package fanbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/currentuser"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/openresource"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	t.Cleanup(localstate.SetConfigFilePathForTest(filepath.Join(home, localstate.AppDataDirName, "config.toml")))
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
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
	server := fanboxmcpserver.New(service)
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

func TestFanboxMCPToolInventoryHasNoAuthTools(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	want := []string{
		"fanbox_current_user", "fanbox_creator", "fanbox_creators", "fanbox_creator_tags",
		"fanbox_creator_posts", "fanbox_tagged_posts", "fanbox_post", "fanbox_home",
		"fanbox_supporting", "fanbox_resolve_url", "fanbox_open_resource",
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("tools=%v want exact registration set %v", names, want)
	}
	for _, name := range names {
		if strings.Contains(name, "auth") || strings.Contains(name, "config") || strings.Contains(name, "cookie") || strings.Contains(name, "browser") || strings.Contains(name, "import") || strings.Contains(name, "login") {
			t.Fatalf("unexpected tool %q in read-only FANBOX server", name)
		}
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
	service, _ := fanboxTestService(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "i.pximg.net" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}, "Content-Length": {"4"}}, Body: io.NopCloser(strings.NewReader("PNG!"))}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	payload, err := json.Marshal(map[string]string{"k": "post_image", "id": "img1", "u": "https://i.pximg.net/img/1.png"})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := sdk.NewResourceRef("fanbox", payload)
	if err != nil {
		t.Fatal(err)
	}
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
