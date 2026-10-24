package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUnifiedMediaSerializationOverIOTransport(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		debug         bool
	}{
		{"normal-2025", "2025-11-25", false}, {"debug-2025", "2025-11-25", true},
		{"normal-2026", "2026-07-28", false}, {"debug-2026", "2026-07-28", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []logging.Event
			var mu sync.Mutex
			ctx := t.Context()
			if tc.debug {
				ctx = logging.WithScope(ctx, logging.SinkFunc(func(event logging.Event) {
					mu.Lock()
					defer mu.Unlock()
					events = append(events, event)
				}), logging.ModulePixivMCP, 0)
			}
			server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, fanbox.SDKPorts{}, nil)
			var calls atomic.Int32
			fixture := &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}}, StructuredContent: countedMediaContent{calls: &calls}}
			expected, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			before := calls.Load()
			for _, name := range []string{"pixiv_artwork_media", "pixiv_gallery_animation", "other"} {
				server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return fixture, nil })
			}
			clientRead, serverWrite := io.Pipe()
			serverRead, clientWrite := io.Pipe()
			t.Cleanup(func() {
				_ = clientRead.Close()
				_ = serverWrite.Close()
				_ = serverRead.Close()
				_ = clientWrite.Close()
			})
			serverSession, err := server.Connect(ctx, &mcp.IOTransport{Reader: serverRead, Writer: serverWrite}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "io-observation-test", Version: "1"}, nil).Connect(ctx, &mcp.IOTransport{Reader: clientRead, Writer: clientWrite}, &mcp.ClientSessionOptions{ProtocolVersion: tc.version})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			resultLength := 0
			for _, name := range []string{"pixiv_artwork_media", "pixiv_gallery_animation", "other"} {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err != nil {
					t.Fatal(err)
				}
				actual, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				resultLength = len(actual)
				if tc.version == "2026-07-28" {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(actual, &fields); err != nil {
						t.Fatal(err)
					}
					if string(fields["resultType"]) != `"complete"` || !strings.Contains(string(fields["_meta"]), `"name":"pixiv-cli"`) {
						t.Fatalf("SDK completion metadata lost: %s", actual)
					}
					delete(fields, "_meta")
					delete(fields, "resultType")
					actual, err = json.Marshal(fields)
					if err != nil {
						t.Fatal(err)
					}
				}
				if string(actual) != string(expected) {
					t.Fatalf("%s changed delivered result: %s", name, actual)
				}
			}
			if calls.Load() != before+3 {
				t.Fatalf("result encoded %d times, want 3", calls.Load()-before)
			}
			mu.Lock()
			observed := append([]logging.Event(nil), events...)
			mu.Unlock()
			want := 0
			if tc.debug {
				want = 2
			}
			if len(observed) != want {
				t.Fatalf("serialization events = %d, want %d", len(observed), want)
			}
			for _, event := range observed {
				if event.Operation != "mcp_response_serialization" || event.Kind != logging.EventCompleted || event.Count != resultLength || event.Duration < 0 {
					t.Fatalf("invalid serialization observation: %+v", event)
				}
			}
			safe, err := json.Marshal(observed)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(safe), "signed-url-and-token") {
				t.Fatal("private media leaked into diagnostics")
			}
		})
	}
}

func TestUnifiedToolRegistry(t *testing.T) {
	var pixivCalls, fanboxCalls atomic.Int32
	proxy := "http://proxy.invalid"
	ports := httpRegistryPorts(t)
	ports.Execute = func(_ context.Context, account pixiv.Account, _ func(context.Context, *pixivsdk.Client) (bool, error)) error {
		pixivCalls.Add(1)
		if account.UserID != 72 {
			t.Errorf("Pixiv account = %+v", account)
		}
		return errors.New("fixture Pixiv port")
	}
	server := mcpserver.New(ports, pixiv.Account{UserID: 72}, fanbox.SDKPorts{
		Open: func(_ context.Context, account fanbox.Account) (*fanboxsdk.Client, error) {
			fanboxCalls.Add(1)
			if account.HTTPSProxyOverride != &proxy {
				t.Errorf("FANBOX override = %+v", account)
			}
			return nil, errors.New("fixture FANBOX port")
		},
	}, &proxy)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "registry-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pixivCalls.Load() != 0 || fanboxCalls.Load() != 0 {
		t.Fatal("discovery opens product credentials")
	}
	for _, test := range []struct {
		name, message string
		args          map[string]any
	}{
		{"pixiv_user_detail", "fixture Pixiv port", map[string]any{"user_id": 1}},
		{"fanbox_current_user", "fixture FANBOX port", map[string]any{}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil {
			t.Fatalf("route %s: %v", test.name, err)
		}
		if !result.IsError || len(result.Content) == 0 {
			t.Fatalf("route %s: %+v", test.name, result)
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok || !strings.Contains(text.Text, test.message) {
			t.Fatalf("wrong product route %s: %+v", test.name, result)
		}
	}
	if pixivCalls.Load() != 1 || fanboxCalls.Load() != 1 {
		t.Fatal("product ports are not independent")
	}
	seen := map[string]bool{}
	writes := map[string]struct{ destructive, idempotent bool }{
		"pixiv_account_login_start": {true, false},
		"pixiv_account_list":        {false, true}, "pixiv_account_status": {false, true}, "pixiv_account_use": {false, true},
		"pixiv_bookmark_add": {false, true}, "pixiv_follow_user": {false, true},
		"pixiv_bookmark_remove": {true, true}, "pixiv_unfollow_user": {true, true},
		"pixiv_comment_create": {false, false},
		"pixiv_comment_reply":  {false, false},
		"pixiv_comment_stamp":  {false, false},
		"pixiv_comment_delete": {true, true},
	}
	for _, tool := range tools.Tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %s", tool.Name)
		}
		seen[tool.Name] = true
		if !strings.HasPrefix(tool.Name, "pixiv_") && !strings.HasPrefix(tool.Name, "fanbox_") {
			t.Errorf("unqualified name %s", tool.Name)
		}
		if strings.Contains(tool.Name, "download") {
			t.Errorf("server-side download remains: %s", tool.Name)
		}
		// Check the discovery wire: a nil Go slice becomes null, not a valid required array.
		for name, schema := range map[string]any{"input": tool.InputSchema, "output": tool.OutputSchema} {
			data, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]any
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			if required, present := object["required"]; present {
				if _, ok := required.([]any); !ok {
					t.Errorf("%s %s schema: required must be an array, got %s", tool.Name, name, data)
				}
			}
		}
		a := tool.Annotations
		if a == nil || a.DestructiveHint == nil || a.OpenWorldHint == nil {
			t.Errorf("missing explicit annotations: %s", tool.Name)
			continue
		}
		write, mutates := writes[tool.Name]
		if a.ReadOnlyHint == mutates || *a.DestructiveHint != write.destructive || a.IdempotentHint != ((!mutates || write.idempotent) && tool.Name != "pixiv_gallery_animation") {
			t.Errorf("incorrect effect annotations %s: %+v", tool.Name, a)
		}
		localOnly := tool.Name == "fanbox_resolve_url" || tool.Name == "pixiv_account_list" || tool.Name == "pixiv_account_status" || tool.Name == "pixiv_account_use"
		if *a.OpenWorldHint == localOnly {
			t.Errorf("incorrect network annotation: %s", tool.Name)
		}
		if tool.Name == "pixiv_reverse_search" && !strings.Contains(strings.ToLower(tool.Description), "upload") {
			t.Error("reverse search omits external upload disclosure")
		}
	}
	for _, name := range []string{"pixiv_show_gallery", "pixiv_search_illust", "pixiv_user_detail", "fanbox_current_user", "fanbox_post"} {
		if !seen[name] {
			t.Errorf("missing product tool %s", name)
		}
	}
	for name := range writes {
		if !seen[name] {
			t.Errorf("missing mutation %s", name)
		}
	}
	for _, name := range []string{"search_illust", "download", "download_random_from_recommendation", "pixiv_download", "pixiv_artwork_download"} {
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}}); err == nil {
			t.Errorf("removed tool accepted: %s", name)
		}
	}
}

func TestServerImplementationVersionIsProtocolOnlyException(t *testing.T) {
	server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, fanbox.SDKPorts{}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if session.InitializeResult().ServerInfo.Version != "3.0.0" {
		t.Fatalf("serverInfo.version=%q, want 3.0.0", session.InitializeResult().ServerInfo.Version)
	}
}

func TestGalleryResourceAndToolMetadata(t *testing.T) {
	server := mcpserver.New(httpRegistryPorts(t), pixiv.Account{}, fanbox.SDKPorts{}, nil)
	c, s := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), s, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "no-ui-fixture", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	resources, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, resource := range resources.Resources {
		if resource.URI == "ui://pixiv-cli/gallery" {
			found = true
			if resource.MIMEType != "text/html;profile=mcp-app" {
				t.Fatal(resource.MIMEType)
			}
		}
	}
	if !found {
		t.Fatal("missing standard Gallery resource even without UI capability")
	}
	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ui://pixiv-cli/gallery"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("contents: %+v", result)
	}
	content := result.Contents[0]
	if content.MIMEType != "text/html;profile=mcp-app" || !strings.Contains(content.Text, "ui/initialize") {
		t.Fatal("missing embedded bridge")
	}
	raw, err := json.Marshal(content.Meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "connectDomains") || !strings.Contains(string(raw), "blob:") {
		t.Fatal("missing explicit local-only CSP")
	}
	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	visual := map[string]bool{"pixiv_show_gallery": true, "pixiv_show_artwork": true, "pixiv_artwork_media": true}
	for _, tool := range tools.Tools {
		raw, err := json.Marshal(tool.Meta)
		if err != nil {
			t.Fatal(err)
		}
		linked := strings.Contains(string(raw), "ui://pixiv-cli/gallery")
		if linked != visual[tool.Name] {
			t.Errorf("unexpected gallery association %s: %s", tool.Name, raw)
		}
		if visual[tool.Name] && tool.OutputSchema == nil {
			t.Errorf("lost no-UI structured fallback: %s", tool.Name)
		}
	}
}

func httpRegistryPorts(t *testing.T) pixiv.SDKPorts {
	t.Helper()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if _, err := store.Init(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	media, err := mediaproxy.New("https://media.test", store, nil)
	if err != nil {
		t.Fatal(err)
	}
	login, err := accounts.NewLogin(t.Context(), "https://media.test/pixiv-login", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{}, errors.New("fixture unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(login.Close)
	return pixiv.SDKPorts{Media: media, Accounts: accounts.Selection{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) { return accounts.LocalSnapshot{}, nil }}, Login: login}
}

func TestConsolidatedToolRegistry(t *testing.T) {
	server := mcpserver.New(httpRegistryPorts(t), pixiv.Account{}, fanbox.SDKPorts{}, nil)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "consolidated-registry", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"pixiv_comments", "pixiv_comment_create", "pixiv_comment_reply", "pixiv_comment_stamp", "pixiv_comment_delete", "pixiv_bookmark_add", "pixiv_bookmark_remove", "fanbox_creator_posts", "fanbox_feed", "pixiv_user_bookmarks", "pixiv_user_relations", "pixiv_timeline_following", "pixiv_timeline_latest", "pixiv_mypixiv"} {
		if !names[name] {
			t.Errorf("missing unified tool %s", name)
		}
	}
	for _, name := range []string{"fanbox_home", "fanbox_supporting", "fanbox_tagged_posts", "pixiv_add_bookmark", "pixiv_add_novel_bookmark", "pixiv_create_artwork_comment", "pixiv_create_novel_comment", "pixiv_delete_artwork_comment", "pixiv_delete_novel_comment", "pixiv_illust_comments", "pixiv_mypixiv_illusts", "pixiv_mypixiv_novels", "pixiv_mypixiv_users", "pixiv_novel_comments", "pixiv_related_users", "pixiv_remove_bookmark", "pixiv_remove_novel_bookmark", "pixiv_reply_artwork_comment", "pixiv_reply_novel_comment", "pixiv_stamp_artwork_comment", "pixiv_stamp_novel_comment", "pixiv_timeline_illust_following", "pixiv_timeline_illust_latest", "pixiv_timeline_novel_following", "pixiv_timeline_novel_latest", "pixiv_user_followers", "pixiv_user_following", "pixiv_user_novel_bookmarks"} {
		if names[name] {
			t.Errorf("retired tool remains registered: %s", name)
		}
	}
	if len(result.Tools) != 54 {
		t.Errorf("HTTP tools=%d, want 54", len(result.Tools))
	}
}
