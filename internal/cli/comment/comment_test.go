package comment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

func TestNewDeclaresCommentInputAndTypeFlags(t *testing.T) {
	cmd := newTestCommand(testData{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "comment ID" {
		t.Fatalf("unexpected comment use: %q", cmd.Use)
	}
	if cmd.Flags().Lookup("type") == nil || cmd.Flags().Lookup("ndjson") == nil {
		t.Fatal("comment command did not register required type/ndjson flags")
	}
}

func TestNewRegistersCommentActionsWithoutChangingReadRoute(t *testing.T) {
	cmd := newTestCommand(testData{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})

	got := make(map[string]bool)
	for _, child := range cmd.Commands() {
		got[child.Name()] = true
	}
	for _, name := range []string{"create", "delete", "reply", "stamp", "stamps"} {
		if !got[name] {
			t.Fatalf("comment command did not register %q action; children = %v", name, cmd.Commands())
		}
	}
	if len(got) != 5 {
		t.Fatalf("comment command registered unexpected actions: %v", got)
	}
}

func TestCommentCreateReturnsResponseCommentID(t *testing.T) {
	transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return commentJSONResponse(req, `{"comment_id":73}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	output := &bytes.Buffer{}
	cmd := commentTestData(client, output)
	cmd.SetArgs([]string{"create", "101", "--type", "artwork", "--comment", "hello", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute comment create: %v", err)
	}
	var result struct {
		CommentID int64 `json:"comment_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if result.CommentID != 73 {
		t.Fatalf("comment ID = %d, want 73", result.CommentID)
	}
}

// 五种 action 的输出外形：create/reply 返回 comment_id，stamp 允许空文案，delete 返回 deleted。
func TestCommentMutationsDispatchArtworkAndNovelNamespaces(t *testing.T) {
	for _, test := range []struct {
		name        string
		args        []string
		response    string
		wantID      int64
		wantDeleted bool
	}{
		{
			name:     "artwork create",
			args:     []string{"create", "101", "--type", "artwork", "--comment", "hello", "--json"},
			response: `{"comment_id":73}`,
			wantID:   73,
		},
		{
			name:     "novel reply",
			args:     []string{"reply", "201", "--type", "novel", "--parent-comment-id", "81", "--comment", "reply", "--json"},
			response: `{"comment_id":83}`,
			wantID:   83,
		},
		{
			name:     "stamp without comment",
			args:     []string{"stamp", "101", "--type", "artwork", "--stamp-id", "9", "--json"},
			response: `{"comment":{"id":86}}`,
			wantID:   86,
		},
		{
			name:        "novel delete",
			args:        []string{"delete", "85", "--type", "novel", "--json"},
			wantDeleted: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if test.wantDeleted {
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Header:     http.Header{},
						Body:       io.NopCloser(strings.NewReader("")),
						Request:    req,
					}, nil
				}
				return commentJSONResponse(req, test.response), nil
			})
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			output := &bytes.Buffer{}
			cmd := commentTestData(client, output)
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute comment mutation: %v", err)
			}
			if test.wantDeleted {
				var result struct {
					Deleted bool `json:"deleted"`
				}
				if err := json.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
				}
				if !result.Deleted {
					t.Fatalf("deleted = false, want true; output=%q", output.String())
				}
				return
			}
			var result struct {
				CommentID int64 `json:"comment_id"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
			}
			if result.CommentID != test.wantID {
				t.Fatalf("comment ID = %d, want %d", result.CommentID, test.wantID)
			}
		})
	}
}

func TestCommentStampsJSONOutputIsSafeAndNonPaginated(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png"
	transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
			t.Fatalf("request = %s %s?%s, want GET /v1/stamps without query", req.Method, req.URL.Path, req.URL.RawQuery)
		}
		return commentJSONResponse(req, `{"stamps":[{"stamp_id":1,"stamp_url":"`+stampURL+`"}]}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	output := &bytes.Buffer{}
	cmd := commentTestData(client, output)
	cmd.SetArgs([]string{"stamps", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute comment stamps: %v", err)
	}
	var result struct {
		Stamps []pixiv.Stamp `json:"stamps"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if len(result.Stamps) != 1 || result.Stamps[0].ID != 1 || result.Stamps[0].Image.Resource.Ref.IsZero() {
		t.Fatalf("stamps = %#v, want one safe stamp with an opaque ref", result.Stamps)
	}
	if strings.Contains(output.String(), stampURL) || strings.Contains(output.String(), "stamp_url") {
		t.Fatalf("stamp output leaked runtime locator: %s", output.String())
	}
}

func TestCommentReadRouteRemainsArtworkAndNovelComments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		body string
	}{
		{
			name: "artwork",
			args: []string{"123", "--type", "artwork", "--json"},
			body: `{"comments":[{"id":8,"comment":"hello","date":"2026-01-02T03:04:05+00:00","user":{"id":7,"name":"artist"}}],"comment_access_control":0,"next_url":null}`,
		},
		{
			name: "novel",
			args: []string{"456", "--type", "novel", "--json"},
			body: `{"comments":[{"id":8,"comment":"hello","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"artist"}}],"next_url":null}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return commentJSONResponse(req, test.body), nil
			})
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			output := &bytes.Buffer{}
			cmd := commentTestData(client, output)
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute comment read: %v", err)
			}
			var result struct {
				Comments      []pixiv.Comment `json:"comments"`
				AccessControl map[string]any  `json:"access_control"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
			}
			if len(result.Comments) != 1 || result.Comments[0].ID != 8 || result.Comments[0].Comment != "hello" {
				t.Fatalf("comments = %#v, want the existing comment envelope", result.Comments)
			}
			if test.name == "artwork" && result.AccessControl["comment_access_control"] != float64(0) {
				t.Fatalf("artwork access_control = %#v, want opaque numeric value", result.AccessControl)
			}
		})
	}
}

func TestCommentMutationsRejectInvalidTypeInputBeforeOpeningClient(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "create missing type", args: []string{"create", "101", "--comment", "hello"}, want: "--type is required for comment"},
		{name: "reply invalid type", args: []string{"reply", "101", "--type", "all", "--parent-comment-id", "1", "--comment", "hello"}, want: "type must be one of artwork, novel"},
		{name: "reply missing parent", args: []string{"reply", "101", "--type", "artwork", "--comment", "hello"}, want: "--parent-comment-id must be positive"},
		{name: "stamp missing stamp", args: []string{"stamp", "101", "--type", "artwork", "--comment", "hello"}, want: "--stamp-id must be positive"},
		{name: "delete URL", args: []string{"delete", "https://www.pixiv.net/artworks/101", "--type", "artwork"}, want: "URL kind is not allowed for this command"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opened := false
			cmd := newTestCommand(testData{
				Input:       strings.NewReader(""),
				Output:      &bytes.Buffer{},
				ErrorOutput: &bytes.Buffer{},
				UsageError:  func(err error) error { return err },
				JSONOut:     func(*bool) (bool, error) { return false, nil },
				Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					opened = true
					return nil
				},
			})
			cmd.SetArgs(test.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if opened {
				t.Fatal("opened SDK client before validating comment input")
			}
		})
	}
}

type commentRoundTripFunc func(*http.Request) (*http.Response, error)

func (f commentRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func commentJSONResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func commentTestData(client *pixiv.Client, output io.Writer) *cobra.Command {
	return newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut: func(override *bool) (bool, error) {
			if override == nil {
				return false, nil
			}
			return *override, nil
		},
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
}

type testData struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	JSONOut     func(*bool) (bool, error)
	Pooled      func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return New(env, pixivcmd.Executor(d.Pooled))
}
