package pixiv_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type novelCommentMutationOutput struct {
	Success   bool   `json:"success"`
	Action    string `json:"action"`
	NovelID   int64  `json:"novel_id,omitempty"`
	CommentID int64  `json:"comment_id,omitempty"`
	Text      string `json:"text"`
}

func TestNovelCommentMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name          string
		args          map[string]any
		wantAction    string
		wantText      string
		wantCommentID int64
	}{
		{
			name:          "create_novel_comment",
			args:          map[string]any{"novel_id": 9, "comment": "hello"},
			wantAction:    "create_novel_comment",
			wantText:      "Created comment on novel 9.",
			wantCommentID: 901,
		},
		{
			name:          "reply_novel_comment",
			args:          map[string]any{"novel_id": 9, "comment": "reply", "parent_comment_id": 11},
			wantAction:    "reply_novel_comment",
			wantText:      "Replied to comment 11 on novel 9.",
			wantCommentID: 902,
		},
		{
			name:          "stamp_novel_comment",
			args:          map[string]any{"novel_id": 9, "comment": "stamp", "stamp_id": 7},
			wantAction:    "stamp_novel_comment",
			wantText:      "Added stamp comment to novel 9.",
			wantCommentID: 903,
		},
		{
			name:          "delete_novel_comment",
			args:          map[string]any{"comment_id": 12},
			wantAction:    "delete_novel_comment",
			wantText:      "Deleted comment 12.",
			wantCommentID: 12,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			if result.IsError {
				t.Fatalf("mutation failed: %+v", result)
			}
			var out novelCommentMutationOutput
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.wantAction || out.Text != test.wantText || out.CommentID != test.wantCommentID {
				t.Fatalf("mutation output = %+v, want action=%q text=%q comment_id=%d", out, test.wantAction, test.wantText, test.wantCommentID)
			}
			if out.NovelID != 9 && test.name != "delete_novel_comment" {
				t.Fatalf("novel mutation output = %+v", out)
			}
		})
	}

	if client.createNovelCommentRequest != (pixiv.PostNovelCommentRequest{NovelID: 9, Comment: "hello"}) {
		t.Fatalf("create novel comment request = %+v", client.createNovelCommentRequest)
	}
	if client.replyNovelCommentRequest != (pixiv.ReplyNovelCommentRequest{NovelID: 9, Comment: "reply", ParentCommentID: 11}) {
		t.Fatalf("reply novel comment request = %+v", client.replyNovelCommentRequest)
	}
	if client.stampNovelCommentRequest != (pixiv.StampNovelCommentRequest{NovelID: 9, Comment: "stamp", StampID: 7}) {
		t.Fatalf("stamp novel comment request = %+v", client.stampNovelCommentRequest)
	}
	if client.deleteNovelCommentRequest != (pixiv.DeleteNovelCommentRequest{CommentID: 12}) {
		t.Fatalf("delete novel comment request = %+v", client.deleteNovelCommentRequest)
	}
}

func TestNovelCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay(t *testing.T) {
	client := &fakeSDKClient{createNovelCommentErr: &sdk.Error{
		Product:    "pixiv",
		Operation:  "PostNovelComment",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "create_novel_comment", map[string]any{"novel_id": 41, "comment": "uncertain"})
	if !result.IsError {
		t.Fatalf("typed novel comment failure must be an MCP error: %+v", result)
	}
	var out novelCommentMutationOutput
	decodeStructured(t, result, &out)
	if out.Success || out.NovelID != 41 || out.CommentID != 0 || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured novel comment error = %+v", out)
	}
	if client.novelCommentWireCalls != 1 {
		t.Fatalf("uncertain novel comment mutation replayed %d times", client.novelCommentWireCalls)
	}
}

func TestNovelCommentMutationSchemasExposeOnlyContractInputs(t *testing.T) {
	serverSession, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	tools := map[string]map[string]any{}
	for tool, err := range serverSession.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		if strings.HasSuffix(tool.Name, "_novel_comment") {
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("marshal %s schema: %v", tool.Name, err)
			}
			var schema map[string]any
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatalf("decode %s schema: %v", tool.Name, err)
			}
			tools[tool.Name] = schema
		}
	}

	want := map[string][]string{
		"create_novel_comment": {"novel_id", "comment"},
		"reply_novel_comment":  {"novel_id", "comment", "parent_comment_id"},
		"stamp_novel_comment":  {"novel_id", "comment", "stamp_id"},
		"delete_novel_comment": {"comment_id"},
	}
	for name, required := range want {
		schema, ok := tools[name]
		if !ok {
			t.Fatalf("%s tool is not registered", name)
		}
		if schema["additionalProperties"] != false {
			t.Fatalf("%s schema must reject unknown fields: %#v", name, schema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s schema properties = %#v", name, schema["properties"])
		}
		for _, field := range required {
			if _, ok := properties[field]; !ok {
				t.Fatalf("%s schema missing %q: %#v", name, field, schema)
			}
		}
		for _, field := range []string{"novel_id", "parent_comment_id", "stamp_id", "comment_id"} {
			if _, ok := properties[field]; ok {
				assertSchemaMinimum(t, properties[field].(map[string]any), 1)
			}
		}
		for _, field := range []string{"comment"} {
			if property, ok := properties[field].(map[string]any); ok {
				if got, ok := property["minLength"].(float64); !ok || got != 1 {
					t.Fatalf("%s %s schema minLength=%#v, want 1", name, field, property["minLength"])
				}
			}
		}
		if name != "stamp_novel_comment" {
			if _, ok := properties["stamp_id"]; ok {
				t.Fatalf("%s schema must not expose stamp_id: %#v", name, schema)
			}
		}
		if name != "reply_novel_comment" {
			if _, ok := properties["parent_comment_id"]; ok {
				t.Fatalf("%s schema must not expose parent_comment_id: %#v", name, schema)
			}
		}
	}
}

func TestNovelCommentMutationRejectsInvalidInputBeforeSDKExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{name: "create_novel_comment", args: map[string]any{"novel_id": 0, "comment": "hello"}},
		{name: "reply_novel_comment", args: map[string]any{"novel_id": 9, "comment": "reply", "parent_comment_id": 0}},
		{name: "stamp_novel_comment", args: map[string]any{"novel_id": 9, "comment": "stamp", "stamp_id": 0}},
		{name: "delete_novel_comment", args: map[string]any{"comment_id": 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
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

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.name, Arguments: test.args})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("invalid novel comment input result=%+v err=%v executions=%d", result, err, executions)
			}
			if executions != 0 || client.novelCommentWireCalls != 0 {
				t.Fatalf("invalid novel comment input reached SDK: executions=%d wire_calls=%d result=%+v err=%v", executions, client.novelCommentWireCalls, result, err)
			}
		})
	}
}
