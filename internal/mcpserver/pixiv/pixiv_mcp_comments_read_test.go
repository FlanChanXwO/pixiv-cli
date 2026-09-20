package pixiv_test

import (
	"context"
	"encoding/json"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCommentReadSchemasMatchLegacyContracts(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	for _, name := range []string{"illust_comments", "novel_comments"} {
		t.Run("input/"+name, func(t *testing.T) {
			tool, ok := byName[name]
			if !ok {
				t.Fatalf("tool %q is not registered", name)
			}
			schema := feedSchemaObject(t, name+" input", tool.InputSchema)
			assertSchemaFields(t, schema, []string{"id", "limit", "page"})
			assertSchemaRequired(t, schema, []string{"id"})
			assertSchemaMinimum(t, feedSchemaProperty(t, schema, "id"), 1)
			assertSchemaMinimum(t, feedSchemaProperty(t, schema, "page"), 1)
			assertSchemaMinimum(t, feedSchemaProperty(t, schema, "limit"), 0)
		})

		t.Run("output/"+name, func(t *testing.T) {
			tool, ok := byName[name]
			if !ok {
				t.Fatalf("tool %q is not registered", name)
			}
			schema := feedSchemaObject(t, name+" output", tool.OutputSchema)
			assertSchemaFields(t, schema, []string{"access_control", "comments", "pagination", "total"})
			assertSchemaRequired(t, schema, []string{"comments", "pagination"})
		})
	}
}

func TestCommentReadLegacyJSONReplayPreservesStructuredContracts(t *testing.T) {
	total := int64(3)
	tests := []struct {
		name          string
		fixture       string
		client        *fakeSDKClient
		wantCommentID int64
		wantTotal     int64
		wantArtworkID int64
		wantNovelID   int64
	}{
		{
			name:          "illust_comments",
			fixture:       `{"id":101}`,
			wantCommentID: 11,
			wantTotal:     total,
			wantArtworkID: 101,
			client: &fakeSDKClient{artworkCommentsResult: pixiv.CommentPage{
				Page:          sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 11, Comment: "artwork comment", User: pixiv.User{ID: 4}}}},
				Total:         &total,
				AccessControl: &pixiv.CommentAccessControl{CanComment: true},
			}},
		},
		{
			name:          "novel_comments",
			fixture:       `{"id":201}`,
			wantCommentID: 21,
			wantTotal:     total,
			wantNovelID:   201,
			client: &fakeSDKClient{novelCommentsResult: pixiv.CommentPage{
				Page:          sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 21, Comment: "novel comment", User: pixiv.User{ID: 5}}}},
				Total:         &total,
				AccessControl: &pixiv.CommentAccessControl{CanComment: true},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal([]byte(test.fixture), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			session, closeSession := newSDKTestSession(t, test.client)
			defer closeSession()

			result := callTool(t, session, test.name, fixture)
			if result.IsError {
				t.Fatalf("comment replay result=%+v", result)
			}
			var out outputs.Comments
			decodeStructured(t, result, &out)
			if len(out.Comments) != 1 || out.Comments[0].ID != test.wantCommentID {
				t.Fatalf("comments=%+v", out.Comments)
			}
			if out.Total == nil || *out.Total != test.wantTotal || out.AccessControl == nil || !out.AccessControl.CanComment {
				t.Fatalf("comment metadata=%+v", out)
			}
			if test.wantArtworkID != 0 && test.client.artworkCommentsRequest.ArtworkID != test.wantArtworkID {
				t.Fatalf("artwork request=%+v", test.client.artworkCommentsRequest)
			}
			if test.wantNovelID != 0 && test.client.novelCommentsRequest.NovelID != test.wantNovelID {
				t.Fatalf("novel request=%+v", test.client.novelCommentsRequest)
			}
		})
	}
}

func TestCommentReadKeepsStampBoundary(t *testing.T) {
	tools := connectAndListTools(t)
	byName := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if _, ok := byName["stamps"]; ok {
		t.Fatal("comments task must not add an unfrozen stamps tool")
	}
	for _, name := range []string{"illust_comments", "novel_comments"} {
		schema := feedSchemaObject(t, name+" input", byName[name].InputSchema)
		properties := schema["properties"].(map[string]any)
		if _, ok := properties["stamp_id"]; ok {
			t.Fatalf("%s input schema exposes mutation-only stamp_id: %#v", name, schema)
		}
	}
}

func TestCommentReadRejectsNonPositiveIDBeforeSDKExecution(t *testing.T) {
	for _, name := range []string{"illust_comments", "novel_comments"} {
		t.Run(name, func(t *testing.T) {
			client := &fakeSDKClient{}
			ports := testSDKPorts(t, client)
			executions := 0
			baseExecute := ports.Execute
			ports.Execute = func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				executions++
				return baseExecute(ctx, account, attempt)
			}
			session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
			defer closeSession()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"id": 0}})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid comment id result=%+v err=%v executions=%d", result, err, executions)
			}
			if executions != 0 {
				t.Fatalf("invalid comment id opened SDK executions=%d result=%+v err=%v", executions, result, err)
			}
		})
	}
}

func assertSchemaMinimum(t *testing.T, schema map[string]any, want float64) {
	t.Helper()
	got, ok := schema["minimum"].(float64)
	if !ok || got != want {
		t.Fatalf("schema minimum=%#v, want %v", schema["minimum"], want)
	}
}
