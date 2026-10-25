package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// TestCommentMutations 覆盖 artwork/novel 共用的 postComment：
// namespace、form、响应 ID（comment_id 与嵌套 comment.id）与删除。
func TestCommentMutations(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) (CommentMutationResult, error)
		path   string
		form   map[string]string
		body   string
		hasID  bool
	}{
		{name: "artwork post", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 101, Comment: "artwork comment"})
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "101", "comment": "artwork comment"}, body: `{"comment_id":901}`, hasID: true},
		{name: "artwork reply", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.ReplyArtworkComment(context.Background(), ReplyArtworkCommentRequest{ArtworkID: 101, Comment: "artwork reply", ParentCommentID: 901})
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "101", "comment": "artwork reply", "parent_comment_id": "901"}, body: `{"comment":{"id":902}}`, hasID: true},
		{name: "novel post", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 201, Comment: "novel comment"})
		}, path: "/v1/novel/comment/add", form: map[string]string{"novel_id": "201", "comment": "novel comment"}, body: `{"comment_id":903}`, hasID: true},
		{name: "novel reply", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.ReplyNovelComment(context.Background(), ReplyNovelCommentRequest{NovelID: 201, Comment: "novel reply", ParentCommentID: 903})
		}, path: "/v1/novel/comment/add", form: map[string]string{"novel_id": "201", "comment": "novel reply", "parent_comment_id": "903"}, body: `{"comment_id":904}`, hasID: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, test.body)
			result, err := test.invoke(client)
			if err != nil {
				t.Fatalf("invoke: %v", err)
			}
			if test.hasID && result.CommentID == 0 {
				t.Fatalf("result = %#v, want a comment ID", result)
			}
			request := recorder.request(t, 0)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if request.URL.Path != test.path || len(request.PostForm) != len(test.form) {
				t.Fatalf("request = %q form=%v", request.URL.Path, request.PostForm)
			}
			for key, want := range test.form {
				if got := request.PostForm.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}

	for name, invoke := range map[string]func(*Client) error{
		"artwork delete": func(c *Client) error {
			return c.DeleteArtworkComment(context.Background(), DeleteArtworkCommentRequest{CommentID: 902})
		},
		"novel delete": func(c *Client) error {
			return c.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{CommentID: 904})
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, recorder := newWireClient(t, `{}`)
			if err := invoke(client); err != nil {
				t.Fatalf("delete: %v", err)
			}
			request := recorder.request(t, 0)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			wantPath := "/v1/illust/comment/delete"
			if name == "novel delete" {
				wantPath = "/v1/novel/comment/delete"
			}
			if request.URL.Path != wantPath || len(request.PostForm) != 1 || request.PostForm.Get("comment_id") == "" {
				t.Fatalf("request = %q form=%v", request.URL.Path, request.PostForm)
			}
		})
	}
}

func TestCommentMutationsRejectInvalidInputBeforeNetwork(t *testing.T) {
	client, recorder := newWireClient(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "artwork comment ID", call: func() error {
			_, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{Comment: "body"})
			return err
		}},
		{name: "artwork comment body", call: func() error {
			_, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1})
			return err
		}},
		{name: "artwork reply parent", call: func() error {
			_, err := client.ReplyArtworkComment(context.Background(), ReplyArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
			return err
		}},
		{name: "artwork delete comment ID", call: func() error {
			return client.DeleteArtworkComment(context.Background(), DeleteArtworkCommentRequest{})
		}},
		{name: "novel comment ID", call: func() error {
			_, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{Comment: "body"})
			return err
		}},
		{name: "novel comment body", call: func() error {
			_, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1})
			return err
		}},
		{name: "novel reply parent", call: func() error {
			_, err := client.ReplyNovelComment(context.Background(), ReplyNovelCommentRequest{NovelID: 1, Comment: "body"})
			return err
		}},
		{name: "novel delete comment ID", call: func() error {
			return client.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidArgument)
			}
		})
	}
	if recorder.calls() != 0 {
		t.Fatalf("invalid comment mutation reached upstream %d time(s)", recorder.calls())
	}
}

func TestCommentMutationsRequireResponseID(t *testing.T) {
	for _, body := range []string{`{}`, `{"status":"ok"}`, `{"comment_id":null}`, `{"comment_id":0}`} {
		t.Run(body, func(t *testing.T) {
			client, recorder := newWireClient(t, body)
			if _, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
			if _, err := client.PostNovelComment(context.Background(), PostNovelCommentRequest{NovelID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("error = %v, want malformed response", err)
			}
			if recorder.calls() != 2 {
				t.Fatalf("request count = %d, want 2", recorder.calls())
			}
		})
	}
}

func TestCommentMutationsPropagateTransportFailure(t *testing.T) {
	client, recorder := newWireClient(t)
	recorder.err = errors.New("transport canary")
	if _, err := client.PostArtworkComment(context.Background(), PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"}); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
		t.Fatalf("create error = %v, want UpstreamUnavailable", err)
	}
	if err := client.DeleteNovelComment(context.Background(), DeleteNovelCommentRequest{CommentID: 1}); sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
		t.Fatalf("delete error = %v, want UpstreamUnavailable", err)
	}
}

// TestStampComments 覆盖 stamp 与普通评论共用 postComment 但 form 独立的契约：
// stamp_id 与 comment 各自保留，空 comment 合法，artwork/novel 同形。
func TestStampComments(t *testing.T) {
	for _, test := range []struct {
		name   string
		invoke func(*Client) (CommentMutationResult, error)
		path   string
		form   map[string]string
	}{
		{name: "artwork text", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 123, Comment: "stamp", StampID: 9})
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "123", "comment": "stamp", "stamp_id": "9"}},
		{name: "artwork empty text", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 123, StampID: 9})
		}, path: "/v1/illust/comment/add", form: map[string]string{"illust_id": "123", "comment": "", "stamp_id": "9"}},
		{name: "novel text", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 456, Comment: "stamp", StampID: 9})
		}, path: "/v1/novel/comment/add", form: map[string]string{"novel_id": "456", "comment": "stamp", "stamp_id": "9"}},
		{name: "novel empty text", invoke: func(c *Client) (CommentMutationResult, error) {
			return c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 456, StampID: 9})
		}, path: "/v1/novel/comment/add", form: map[string]string{"novel_id": "456", "comment": "", "stamp_id": "9"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := newWireClient(t, `{"comment_id":901}`)
			result, err := test.invoke(client)
			if err != nil || result.CommentID != 901 {
				t.Fatalf("result = %#v err=%v", result, err)
			}
			request := recorder.request(t, 0)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if request.URL.Path != test.path || len(request.PostForm) != len(test.form) {
				t.Fatalf("request = %q form=%v", request.URL.Path, request.PostForm)
			}
			for key, want := range test.form {
				if got := request.PostForm.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}

	for _, test := range []struct {
		name   string
		invoke func(*Client) error
	}{
		{name: "artwork missing artwork", invoke: func(c *Client) error {
			_, err := c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{Comment: "body", StampID: 9})
			return err
		}},
		{name: "artwork missing stamp", invoke: func(c *Client) error {
			_, err := c.StampArtworkComment(context.Background(), StampArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
			return err
		}},
		{name: "novel missing novel", invoke: func(c *Client) error {
			_, err := c.StampNovelComment(context.Background(), StampNovelCommentRequest{Comment: "body", StampID: 9})
			return err
		}},
		{name: "novel missing stamp", invoke: func(c *Client) error {
			_, err := c.StampNovelComment(context.Background(), StampNovelCommentRequest{NovelID: 1, Comment: "body"})
			return err
		}},
	} {
		t.Run("invalid/"+test.name, func(t *testing.T) {
			client, recorder := newWireClient(t)
			if err := test.invoke(client); sdk.ReasonOf(err) != sdk.InvalidArgument || recorder.calls() != 0 {
				t.Fatalf("reason = %q calls = %d", sdk.ReasonOf(err), recorder.calls())
			}
		})
	}
}

func TestStampsMapImageResources(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
			t.Fatalf("unexpected stamps request: %s", req.URL)
		}
		return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	stamps, err := client.Stamps(context.Background(), StampsRequest{})
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	if len(stamps) != 1 || stamps[0].ID != 1 || stamps[0].Image.Resource.URL != stampURL || stamps[0].Image.Resource.Ref.IsZero() {
		t.Fatalf("stamps = %#v", stamps)
	}
	payload, err := sdk.ResourceRefPayload(stamps[0].Image.Resource.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if !strings.Contains(string(payload), `"k":"stamp"`) || !strings.Contains(string(payload), `"id":1`) {
		t.Fatalf("stamp resource payload = %s", payload)
	}
}

func TestStampsRejectMalformedResponse(t *testing.T) {
	for name, body := range map[string]string{
		"missing list":      `{}`,
		"null list":         `{"stamps":null}`,
		"zero id":           `{"stamps":[{"stamp_id":0,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		"negative id":       `{"stamps":[{"stamp_id":-1,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		"missing url":       `{"stamps":[{"stamp_id":1}]}`,
		"plain http":        `{"stamps":[{"stamp_id":1,"stamp_url":"http://s.pximg.net/stamp.png"}]}`,
		"root path":         `{"stamps":[{"stamp_id":1,"stamp_url":"https://s.pximg.net"}]}`,
		"userinfo":          `{"stamps":[{"stamp_id":1,"stamp_url":"https://user:pass@s.pximg.net/stamp.png"}]}`,
		"untrusted host":    `{"stamps":[{"stamp_id":1,"stamp_url":"https://example.com/stamp.png"}]}`,
		"non-null next_url": `{"stamps":[],"next_url":"https://app-api.pixiv.net/v1/stamps?offset=40"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(body), nil })
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if _, err := client.Stamps(context.Background(), StampsRequest{}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("Stamps reason = %q, err = %v", sdk.ReasonOf(err), err)
			}
		})
	}

	client, _ := newWireClient(t, `{"stamps":[]}`)
	stamps, err := client.Stamps(context.Background(), StampsRequest{})
	if err != nil || stamps == nil || len(stamps) != 0 {
		t.Fatalf("empty stamps = %#v err=%v, want non-nil empty slice", stamps, err)
	}
}

func TestStampResourceReferenceCanBeReopenedWithoutEmbeddedURL(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png?signature=sentinel"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "app-api.pixiv.net":
			if req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
				return nil, http.ErrNotSupported
			}
			return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
		case "s.pximg.net":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: http.NoBody}, nil
		default:
			return nil, http.ErrNotSupported
		}
	})
	httpClient := &http.Client{Transport: rt}
	first, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith first: %v", err)
	}
	stamps, err := first.Stamps(context.Background(), StampsRequest{})
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	second, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith second: %v", err)
	}
	response, err := second.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: stamps[0].Image.Resource.Ref})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
}

func TestStampJSONIsOutputSafe(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"stamp","id":1}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	stamp := Stamp{
		ID: 1,
		Image: ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://s.pximg.net/stamp.png?token=do-not-leak",
			RequestHeaders: map[string]string{"Cookie": "secret-cookie"},
		}},
	}
	data, err := json.Marshal(stamp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, forbidden := range []string{"s.pximg.net", "do-not-leak", "Cookie", "secret-cookie", "request_headers"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("stamp JSON leaked %q: %s", forbidden, data)
		}
	}
}
