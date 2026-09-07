package comments_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path          string
	query         url.Values
	body          string
	getCalls      int
	mutationPath  string
	mutationForm  url.Values
	mutationBody  string
	mutationErr   error
	postJSONCalls int
	postFormCalls int
	postFormErr   error
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.getCalls++
	f.path = path
	f.query = query
	return json.Unmarshal([]byte(f.body), out)
}

func (f *fakeTransport) PostFormJSON(_ context.Context, path string, form url.Values, out any) error {
	f.postJSONCalls++
	f.mutationPath = path
	f.mutationForm = form
	if f.mutationErr != nil {
		return f.mutationErr
	}
	return json.Unmarshal([]byte(f.mutationBody), out)
}

func (f *fakeTransport) PostForm(_ context.Context, path string, form url.Values) error {
	f.postFormCalls++
	f.mutationPath = path
	f.mutationForm = form
	return f.postFormErr
}

func TestCommentsMapsParentMetadataAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7},"parent_comment":{"id":6,"caption":"parent","created_at":"2024-01-01T03:04:05+00:00","user":{"id":5}}}],"total_comments":2,"access_control":{"can_comment":false,"is_locked":true},"next_url":"https://app-api.pixiv.net/v2/novel/comments?novel_id=123&offset=20"}`}
	result, err := comments.New(transport).List(context.Background(), comments.Request{NovelID: 123, Offset: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v2/novel/comments" || transport.query.Get("novel_id") != "123" || transport.query.Get("offset") != "10" || len(result.Items) != 1 || result.Items[0].Comment != "child" || result.Items[0].ParentComment == nil || result.Items[0].ParentComment.Comment != "parent" || result.Total == nil || *result.Total != 2 || result.AccessControl == nil || !result.AccessControl.IsLocked || !result.HasNext || result.NextOffset != 20 {
		t.Fatalf("result = %#v request=%q %v", result, transport.path, transport.query)
	}
}

func TestCommentsRejectsInvalidParentID(t *testing.T) {
	_, err := comments.New(&fakeTransport{body: `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`}).List(context.Background(), comments.Request{NovelID: 1})
	if err == nil {
		t.Fatal("invalid parent comment unexpectedly succeeded")
	}
}

func TestCommentsListValidatesRequestAndRequiresCommentsList(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		request            comments.Request
		wantMalformed      bool
		wantEmptyItems     bool
		wantNoTransportHit bool
	}{
		{
			name:               "non-positive novel ID",
			body:               `{"comments":[]}`,
			request:            comments.Request{NovelID: 0},
			wantNoTransportHit: true,
		},
		{
			name:               "negative offset",
			body:               `{"comments":[]}`,
			request:            comments.Request{NovelID: 1, Offset: -1},
			wantNoTransportHit: true,
		},
		{
			name:          "missing comments",
			body:          `{}`,
			request:       comments.Request{NovelID: 1},
			wantMalformed: true,
		},
		{
			name:          "null comments",
			body:          `{"comments":null}`,
			request:       comments.Request{NovelID: 1},
			wantMalformed: true,
		},
		{
			name:           "empty comments",
			body:           `{"comments":[]}`,
			request:        comments.Request{NovelID: 1},
			wantEmptyItems: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: test.body}
			result, err := comments.New(transport).List(context.Background(), test.request)
			if test.wantMalformed {
				if !errors.Is(err, protocol.ErrMalformedResponse) {
					t.Fatalf("List error = %v, want malformed response", err)
				}
			} else if test.wantNoTransportHit {
				if err == nil {
					t.Fatal("invalid request unexpectedly succeeded")
				}
				if transport.getCalls != 0 {
					t.Fatalf("GetJSON calls = %d, want 0", transport.getCalls)
				}
			} else if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if test.wantEmptyItems && (err != nil || result.Items == nil || len(result.Items) != 0) {
				t.Fatalf("empty result = %#v, err=%v", result, err)
			}
		})
	}
}

func TestCommentsCreateBuildsFormAndMapsCommentID(t *testing.T) {
	transport := &fakeTransport{mutationBody: `{"comment_id":73}`}
	result, err := comments.New(transport).Create(context.Background(), comments.CreateRequest{
		NovelID: 123,
		Comment: "hello",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.CommentID != 73 {
		t.Fatalf("CommentID = %d, want 73", result.CommentID)
	}
	if transport.postJSONCalls != 1 || transport.mutationPath != "/v1/novel/comment/add" {
		t.Fatalf("mutation request = calls:%d path:%q", transport.postJSONCalls, transport.mutationPath)
	}
	if transport.mutationForm.Get("novel_id") != "123" || transport.mutationForm.Get("comment") != "hello" || len(transport.mutationForm) != 2 {
		t.Fatalf("mutation form = %#v", transport.mutationForm)
	}
}

func TestCommentsReplyBuildsFormAndMapsCommentID(t *testing.T) {
	transport := &fakeTransport{mutationBody: `{"comment_id":74}`}
	result, err := comments.New(transport).Reply(context.Background(), comments.ReplyRequest{
		NovelID:         123,
		Comment:         "reply",
		ParentCommentID: 6,
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if result.CommentID != 74 {
		t.Fatalf("CommentID = %d, want 74", result.CommentID)
	}
	if transport.postJSONCalls != 1 || transport.mutationPath != "/v1/novel/comment/add" {
		t.Fatalf("mutation request = calls:%d path:%q", transport.postJSONCalls, transport.mutationPath)
	}
	if transport.mutationForm.Get("novel_id") != "123" || transport.mutationForm.Get("comment") != "reply" || transport.mutationForm.Get("parent_comment_id") != "6" || len(transport.mutationForm) != 3 {
		t.Fatalf("mutation form = %#v", transport.mutationForm)
	}
}

func TestCommentsStampBuildsFormAndMapsCommentID(t *testing.T) {
	transport := &fakeTransport{mutationBody: `{"comment_id":75}`}
	result, err := comments.New(transport).Stamp(context.Background(), comments.StampRequest{
		NovelID: 123,
		Comment: "stamp",
		StampID: 9,
	})
	if err != nil {
		t.Fatalf("Stamp returned error: %v", err)
	}
	if result.CommentID != 75 {
		t.Fatalf("CommentID = %d, want 75", result.CommentID)
	}
	if transport.postJSONCalls != 1 || transport.mutationPath != "/v1/novel/comment/add" {
		t.Fatalf("mutation request = calls:%d path:%q", transport.postJSONCalls, transport.mutationPath)
	}
	if transport.mutationForm.Get("novel_id") != "123" || transport.mutationForm.Get("comment") != "stamp" || transport.mutationForm.Get("stamp_id") != "9" || len(transport.mutationForm) != 3 {
		t.Fatalf("mutation form = %#v", transport.mutationForm)
	}
}

func TestCommentsDeleteBuildsForm(t *testing.T) {
	transport := &fakeTransport{}
	if err := comments.New(transport).Delete(context.Background(), 76); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if transport.postFormCalls != 1 || transport.mutationPath != "/v1/novel/comment/delete" {
		t.Fatalf("mutation request = calls:%d path:%q", transport.postFormCalls, transport.mutationPath)
	}
	if transport.mutationForm.Get("comment_id") != "76" || len(transport.mutationForm) != 1 {
		t.Fatalf("mutation form = %#v", transport.mutationForm)
	}
}

func TestCommentsMutationsRejectInvalidRequestsBeforeTransport(t *testing.T) {
	tests := []struct {
		name string
		run  func(*fakeTransport) error
	}{
		{
			name: "create non-positive novel ID",
			run: func(transport *fakeTransport) error {
				_, err := comments.New(transport).Create(context.Background(), comments.CreateRequest{NovelID: 0, Comment: "body"})
				return err
			},
		},
		{
			name: "create empty body",
			run: func(transport *fakeTransport) error {
				_, err := comments.New(transport).Create(context.Background(), comments.CreateRequest{NovelID: 1})
				return err
			},
		},
		{
			name: "reply non-positive parent ID",
			run: func(transport *fakeTransport) error {
				_, err := comments.New(transport).Reply(context.Background(), comments.ReplyRequest{NovelID: 1, Comment: "body"})
				return err
			},
		},
		{
			name: "stamp non-positive stamp ID",
			run: func(transport *fakeTransport) error {
				_, err := comments.New(transport).Stamp(context.Background(), comments.StampRequest{NovelID: 1, Comment: "body"})
				return err
			},
		},
		{
			name: "delete non-positive comment ID",
			run: func(transport *fakeTransport) error {
				return comments.New(transport).Delete(context.Background(), 0)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{mutationBody: `{"comment_id":73}`}
			if err := test.run(transport); err == nil {
				t.Fatal("invalid request unexpectedly succeeded")
			}
			if transport.postJSONCalls != 0 || transport.postFormCalls != 0 {
				t.Fatalf("transport calls = post_json:%d post_form:%d, want 0", transport.postJSONCalls, transport.postFormCalls)
			}
		})
	}
}

func TestCommentsCreateRejectsMalformedCommentID(t *testing.T) {
	for _, body := range []string{`{}`, `{"comment_id":null}`, `{"comment_id":0}`, `{"comment_id":-1}`} {
		t.Run(body, func(t *testing.T) {
			transport := &fakeTransport{mutationBody: body}
			result, err := comments.New(transport).Create(context.Background(), comments.CreateRequest{NovelID: 1, Comment: "body"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Create error = %v, want malformed response", err)
			}
			if result.CommentID != 0 || transport.postJSONCalls != 1 {
				t.Fatalf("result=%#v post_json_calls=%d", result, transport.postJSONCalls)
			}
		})
	}
}

func TestCommentsMutationsPropagateTransportErrors(t *testing.T) {
	wantErr := errors.New("transport canary")
	postJSONTransport := &fakeTransport{mutationErr: wantErr}
	if _, err := comments.New(postJSONTransport).Create(context.Background(), comments.CreateRequest{NovelID: 1, Comment: "body"}); !errors.Is(err, wantErr) {
		t.Fatalf("Create error = %v, want transport error", err)
	}

	postFormTransport := &fakeTransport{postFormErr: wantErr}
	if err := comments.New(postFormTransport).Delete(context.Background(), 1); !errors.Is(err, wantErr) {
		t.Fatalf("Delete error = %v, want transport error", err)
	}
}
