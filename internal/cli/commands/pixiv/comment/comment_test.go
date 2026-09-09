package comment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestNewDeclaresCommentInputAndTypeFlags(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "comment ID" {
		t.Fatalf("unexpected comment use: %q", cmd.Use)
	}
	if cmd.Flags().Lookup("type") == nil || cmd.Flags().Lookup("ndjson") == nil {
		t.Fatal("comment command did not register required type/ndjson flags")
	}
}

func TestNewRegistersCommentActionsWithoutChangingReadRoute(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})

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
	var request *http.Request
	transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		request = req
		if req.Method != http.MethodPost || req.URL.Path != "/v1/illust/comment/add" {
			t.Fatalf("request = %s %s, want POST /v1/illust/comment/add", req.Method, req.URL.Path)
		}
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		if req.PostForm.Get("illust_id") != "101" || req.PostForm.Get("comment") != "hello" || len(req.PostForm) != 2 {
			t.Fatalf("form = %#v, want illust_id=101 and comment=hello", req.PostForm)
		}
		return commentJSONResponse(req, `{"comment_id":73}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	output := &bytes.Buffer{}
	cmd := New(commentTestData(client, output))
	cmd.SetArgs([]string{"create", "101", "--type", "artwork", "--comment", "hello", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute comment create: %v", err)
	}
	if request == nil {
		t.Fatal("comment create did not send a request")
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

func TestCommentMutationsDispatchArtworkAndNovelNamespaces(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		path         string
		form         map[string]string
		response     string
		wantID       int64
		wantDeleted  bool
		responseCode int
	}{
		{
			name:     "novel create",
			args:     []string{"create", "201", "--type", "novel", "--comment", "hello", "--json"},
			path:     "/v1/novel/comment/add",
			form:     map[string]string{"novel_id": "201", "comment": "hello"},
			response: `{"comment_id":81}`,
			wantID:   81,
		},
		{
			name:     "artwork reply",
			args:     []string{"reply", "101", "--type", "artwork", "--parent-comment-id", "73", "--comment", "reply", "--json"},
			path:     "/v1/illust/comment/add",
			form:     map[string]string{"illust_id": "101", "comment": "reply", "parent_comment_id": "73"},
			response: `{"comment_id":82}`,
			wantID:   82,
		},
		{
			name:     "novel reply",
			args:     []string{"reply", "201", "--type", "novel", "--parent-comment-id", "81", "--comment", "reply", "--json"},
			path:     "/v1/novel/comment/add",
			form:     map[string]string{"novel_id": "201", "comment": "reply", "parent_comment_id": "81"},
			response: `{"comment_id":83}`,
			wantID:   83,
		},
		{
			name:     "artwork stamp",
			args:     []string{"stamp", "101", "--type", "artwork", "--stamp-id", "9", "--comment", "stamp", "--json"},
			path:     "/v1/illust/comment/add",
			form:     map[string]string{"illust_id": "101", "comment": "stamp", "stamp_id": "9"},
			response: `{"comment_id":84}`,
			wantID:   84,
		},
		{
			name:     "novel stamp",
			args:     []string{"stamp", "201", "--type", "novel", "--stamp-id", "9", "--comment", "stamp", "--json"},
			path:     "/v1/novel/comment/add",
			form:     map[string]string{"novel_id": "201", "comment": "stamp", "stamp_id": "9"},
			response: `{"comment_id":85}`,
			wantID:   85,
		},
		{
			name:         "artwork delete",
			args:         []string{"delete", "84", "--type", "artwork", "--json"},
			path:         "/v1/illust/comment/delete",
			form:         map[string]string{"comment_id": "84"},
			wantDeleted:  true,
			responseCode: http.StatusNoContent,
		},
		{
			name:         "novel delete",
			args:         []string{"delete", "85", "--type", "novel", "--json"},
			path:         "/v1/novel/comment/delete",
			form:         map[string]string{"comment_id": "85"},
			wantDeleted:  true,
			responseCode: http.StatusNoContent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.URL.Path != test.path {
					t.Fatalf("request = %s %s, want POST %s", req.Method, req.URL.Path, test.path)
				}
				if err := req.ParseForm(); err != nil {
					return nil, err
				}
				if len(req.PostForm) != len(test.form) {
					t.Fatalf("form = %#v, want %#v", req.PostForm, test.form)
				}
				for key, want := range test.form {
					if got := req.PostForm.Get(key); got != want {
						t.Fatalf("form[%q] = %q, want %q", key, got, want)
					}
				}
				if test.wantDeleted {
					return &http.Response{
						StatusCode: test.responseCode,
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
			cmd := New(commentTestData(client, output))
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
	cmd := New(commentTestData(client, output))
	cmd.SetArgs([]string{"stamps", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute comment stamps: %v", err)
	}
	var result struct {
		Stamps []pixiv.StampDTO `json:"stamps"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if len(result.Stamps) != 1 || result.Stamps[0].ID != 1 || result.Stamps[0].Image.Resource == nil {
		t.Fatalf("stamps = %#v, want one safe stamp DTO", result.Stamps)
	}
	if strings.Contains(output.String(), stampURL) || strings.Contains(output.String(), "stamp_url") {
		t.Fatalf("stamp output leaked runtime locator: %s", output.String())
	}
}

func TestCommentReadRouteRemainsArtworkAndNovelComments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
		form string
	}{
		{
			name: "artwork",
			args: []string{"123", "--type", "artwork", "--json"},
			path: "/v3/illust/comments",
			form: "illust_id=123",
		},
		{
			name: "novel",
			args: []string{"456", "--type", "novel", "--json"},
			path: "/v2/novel/comments",
			form: "novel_id=456",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := commentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.Path != test.path || req.URL.RawQuery != test.form {
					t.Fatalf("request = %s %s?%s, want GET %s?%s", req.Method, req.URL.Path, req.URL.RawQuery, test.path, test.form)
				}
				return commentJSONResponse(req, `{"comments":[{"id":8,"comment":"hello","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"artist"}}],"next_url":null}`), nil
			})
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			output := &bytes.Buffer{}
			cmd := New(commentTestData(client, output))
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute comment read: %v", err)
			}
			var result struct {
				Comments []pixiv.CommentDTO `json:"comments"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
			}
			if len(result.Comments) != 1 || result.Comments[0].ID != 8 || result.Comments[0].Comment != "hello" {
				t.Fatalf("comments = %#v, want the existing comment envelope", result.Comments)
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
			cmd := New(deps.Data{
				Input:       strings.NewReader(""),
				Output:      &bytes.Buffer{},
				ErrorOutput: &bytes.Buffer{},
				UsageError:  func(err error) error { return err },
				JSONOut:     func(*bool) (bool, error) { return false, nil },
				Pooled: func(context.Context, deps.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
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

func commentTestData(client *pixiv.Client, output io.Writer) deps.Data {
	return deps.Data{
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
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	}
}
