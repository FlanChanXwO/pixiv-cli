package mcpserver_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUnifiedToolRegistry(t *testing.T) {
	var pixivCalls, fanboxCalls atomic.Int32
	proxy := "http://proxy.invalid"
	server := mcpserver.New(pixiv.SDKPorts{
		Execute: func(_ context.Context, account pixiv.Account, _ func(context.Context, *pixivsdk.Client) (bool, error)) error {
			pixivCalls.Add(1)
			if account.UserID != 72 {
				t.Errorf("Pixiv account = %+v", account)
			}
			return errors.New("fixture Pixiv port")
		},
	}, pixiv.Account{UserID: 72}, fanbox.SDKPorts{
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
		"pixiv_account_list": {false, true}, "pixiv_account_status": {false, true}, "pixiv_account_use": {false, true},
		"pixiv_add_bookmark": {false, true}, "pixiv_add_novel_bookmark": {false, true}, "pixiv_follow_user": {false, true},
		"pixiv_remove_bookmark": {true, true}, "pixiv_remove_novel_bookmark": {true, true}, "pixiv_unfollow_user": {true, true},
		"pixiv_create_artwork_comment": {false, false}, "pixiv_create_novel_comment": {false, false},
		"pixiv_reply_artwork_comment": {false, false}, "pixiv_reply_novel_comment": {false, false},
		"pixiv_stamp_artwork_comment": {false, false}, "pixiv_stamp_novel_comment": {false, false},
		"pixiv_delete_artwork_comment": {true, true}, "pixiv_delete_novel_comment": {true, true},
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
		a := tool.Annotations
		if a == nil || a.DestructiveHint == nil || a.OpenWorldHint == nil {
			t.Errorf("missing explicit annotations: %s", tool.Name)
			continue
		}
		write, mutates := writes[tool.Name]
		if a.ReadOnlyHint == mutates || *a.DestructiveHint != write.destructive || a.IdempotentHint != (!mutates || write.idempotent) {
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
	for _, name := range []string{"pixiv_search_illust", "pixiv_user_detail", "fanbox_current_user", "fanbox_post"} {
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
