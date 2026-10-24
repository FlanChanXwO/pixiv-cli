package pixiv_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestExplicitCommentMutationsReturnResponseIDsAndUseNamespaces(t *testing.T) {
	var requests []*http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		requests = append(requests, req)
		switch req.URL.Path {
		case "/v1/illust/comment/add":
			if req.PostForm.Get("parent_comment_id") == "" {
				return jsonResponse(`{"comment_id":901}`), nil
			}
			return jsonResponse(`{"comment_id":902}`), nil
		case "/v1/novel/comment/add":
			if req.PostForm.Get("parent_comment_id") == "" {
				return jsonResponse(`{"comment_id":903}`), nil
			}
			return jsonResponse(`{"comment_id":904}`), nil
		case "/v1/illust/comment/delete", "/v1/novel/comment/delete":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	artworkComment, err := client.PostArtworkComment(context.Background(), pixiv.PostArtworkCommentRequest{
		ArtworkID: 101,
		Comment:   "artwork comment",
	})
	if err != nil {
		t.Fatalf("PostArtworkComment: %v", err)
	}
	if artworkComment.CommentID != 901 {
		t.Fatalf("artwork comment ID = %d, want 901", artworkComment.CommentID)
	}

	artworkReply, err := client.ReplyArtworkComment(context.Background(), pixiv.ReplyArtworkCommentRequest{
		ArtworkID:       101,
		Comment:         "artwork reply",
		ParentCommentID: 901,
	})
	if err != nil {
		t.Fatalf("ReplyArtworkComment: %v", err)
	}
	if artworkReply.CommentID != 902 {
		t.Fatalf("artwork reply ID = %d, want 902", artworkReply.CommentID)
	}

	if err := client.DeleteArtworkComment(context.Background(), pixiv.DeleteArtworkCommentRequest{CommentID: artworkReply.CommentID}); err != nil {
		t.Fatalf("DeleteArtworkComment: %v", err)
	}

	novelComment, err := client.PostNovelComment(context.Background(), pixiv.PostNovelCommentRequest{
		NovelID: 201,
		Comment: "novel comment",
	})
	if err != nil {
		t.Fatalf("PostNovelComment: %v", err)
	}
	if novelComment.CommentID != 903 {
		t.Fatalf("novel comment ID = %d, want 903", novelComment.CommentID)
	}

	novelReply, err := client.ReplyNovelComment(context.Background(), pixiv.ReplyNovelCommentRequest{
		NovelID:         201,
		Comment:         "novel reply",
		ParentCommentID: 903,
	})
	if err != nil {
		t.Fatalf("ReplyNovelComment: %v", err)
	}
	if novelReply.CommentID != 904 {
		t.Fatalf("novel reply ID = %d, want 904", novelReply.CommentID)
	}

	if err := client.DeleteNovelComment(context.Background(), pixiv.DeleteNovelCommentRequest{CommentID: novelReply.CommentID}); err != nil {
		t.Fatalf("DeleteNovelComment: %v", err)
	}

	if len(requests) != 6 {
		t.Fatalf("request count = %d, want 6", len(requests))
	}
	checks := []struct {
		request *http.Request
		path    string
		form    map[string]string
	}{
		{requests[0], "/v1/illust/comment/add", map[string]string{"illust_id": "101", "comment": "artwork comment"}},
		{requests[1], "/v1/illust/comment/add", map[string]string{"illust_id": "101", "comment": "artwork reply", "parent_comment_id": "901"}},
		{requests[2], "/v1/illust/comment/delete", map[string]string{"comment_id": "902"}},
		{requests[3], "/v1/novel/comment/add", map[string]string{"novel_id": "201", "comment": "novel comment"}},
		{requests[4], "/v1/novel/comment/add", map[string]string{"novel_id": "201", "comment": "novel reply", "parent_comment_id": "903"}},
		{requests[5], "/v1/novel/comment/delete", map[string]string{"comment_id": "904"}},
	}
	for _, check := range checks {
		if check.request.URL.Path != check.path {
			t.Errorf("path = %q, want %q", check.request.URL.Path, check.path)
		}
		for key, want := range check.form {
			if got := check.request.PostForm.Get(key); got != want {
				t.Errorf("%s form[%q] = %q, want %q", check.path, key, got, want)
			}
		}
	}
}

func TestExplicitCommentMutationsRejectInvalidInputsBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "artwork comment ID",
			call: func() error {
				_, err := client.PostArtworkComment(context.Background(), pixiv.PostArtworkCommentRequest{ArtworkID: 0, Comment: "body"})
				return err
			},
		},
		{
			name: "artwork comment body",
			call: func() error {
				_, err := client.PostArtworkComment(context.Background(), pixiv.PostArtworkCommentRequest{ArtworkID: 1})
				return err
			},
		},
		{
			name: "artwork reply parent",
			call: func() error {
				_, err := client.ReplyArtworkComment(context.Background(), pixiv.ReplyArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
				return err
			},
		},
		{
			name: "artwork delete comment ID",
			call: func() error {
				return client.DeleteArtworkComment(context.Background(), pixiv.DeleteArtworkCommentRequest{})
			},
		},
		{
			name: "novel comment ID",
			call: func() error {
				_, err := client.PostNovelComment(context.Background(), pixiv.PostNovelCommentRequest{NovelID: 0, Comment: "body"})
				return err
			},
		},
		{
			name: "novel comment body",
			call: func() error {
				_, err := client.PostNovelComment(context.Background(), pixiv.PostNovelCommentRequest{NovelID: 1})
				return err
			},
		},
		{
			name: "novel reply parent",
			call: func() error {
				_, err := client.ReplyNovelComment(context.Background(), pixiv.ReplyNovelCommentRequest{NovelID: 1, Comment: "body"})
				return err
			},
		},
		{
			name: "novel delete comment ID",
			call: func() error {
				return client.DeleteNovelComment(context.Background(), pixiv.DeleteNovelCommentRequest{})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if reason := sdk.ReasonOf(test.call()); reason != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidArgument)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid comment mutation reached upstream %d time(s)", calls)
	}
}

func TestPostArtworkCommentRequiresResponseCommentID(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(`{"status":"ok"}`), nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	_, err = client.PostArtworkComment(context.Background(), pixiv.PostArtworkCommentRequest{ArtworkID: 1, Comment: "body"})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse, err)
	}
	if calls != 1 {
		t.Fatalf("request count = %d, want 1", calls)
	}
}
