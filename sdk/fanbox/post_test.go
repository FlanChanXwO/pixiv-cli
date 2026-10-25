package fanbox_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// FANBOX creator 与 feed 端点共用 creatorProfile/postList 的响应与 cursor 实现。

type recordedRequest struct{ url *url.URL }

type endpointRecorder struct {
	bodies   []string
	requests []recordedRequest
}

func (r *endpointRecorder) roundTrip(req *http.Request) (*http.Response, error) {
	r.requests = append(r.requests, recordedRequest{url: req.URL})
	if req.URL.Host == "www.fanbox.cc" {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	}
	if len(r.bodies) == 0 {
		return nil, http.ErrNotSupported
	}
	body := r.bodies[0]
	r.bodies = r.bodies[1:]
	return jsonResponse(body), nil
}

func (r *endpointRecorder) lastAPI(t *testing.T) *url.URL {
	t.Helper()
	for index := len(r.requests) - 1; index >= 0; index-- {
		if r.requests[index].url.Host == "api.fanbox.cc" {
			return r.requests[index].url
		}
	}
	t.Fatal("no API request recorded")
	return nil
}

func recorderClient(t *testing.T, bodies ...string) (*fanbox.Client, *endpointRecorder) {
	t.Helper()
	recorder := &endpointRecorder{bodies: bodies}
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(recorder.roundTrip)},
	})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	return client, recorder
}

func TestCreatorEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		invoke   func(*fanbox.Client) error
		path     string
		query    map[string]string
		wantItem string
	}{
		{
			name: "profile", body: `{"body":{"creatorId":"creator value","user":{"name":"Creator Name","iconUrl":"https://i.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://i.pximg.net/cover.png","plan":{"fee":500,"hasSupportingPlan":true}}}`,
			path: "/creator.get", query: map[string]string{"creatorId": "creator value"},
			invoke: func(c *fanbox.Client) error {
				profile, err := c.Creator(context.Background(), fanbox.CreatorRequest{CreatorID: "creator value"})
				if err != nil {
					return err
				}
				if profile.ID != "creator value" || profile.Name != "Creator Name" || profile.PlanFee != 500 || !profile.HasSupportingPlan ||
					profile.Icon.Resource.URL != "https://i.pximg.net/icon.png" || profile.Cover.Resource.URL != "https://i.pximg.net/cover.png" {
					t.Errorf("profile = %+v", profile)
				}
				return nil
			},
		},
		{
			name: "tags", body: `{"body":{"tags":[{"tag":"fanart","url":"https://www.fanbox.cc/@writer/posts/tag/fanart"}]}}`,
			path: "/tag.getFeatured", query: map[string]string{"creatorId": "writer"},
			invoke: func(c *fanbox.Client) error {
				items, err := c.CreatorTags(context.Background(), fanbox.CreatorTagsRequest{CreatorID: "writer"})
				if err == nil && (len(items) != 1 || items[0].Name != "fanart" || items[0].URL == "") {
					t.Errorf("items = %+v", items)
				}
				return err
			},
		},
		{
			name: "tagged posts", body: `{"body":{"posts":[{"id":"tagged","title":"tagged","publishedDatetime":"2024-01-01T00:00:00Z"}]}}`,
			path: "/post.listTagged", query: map[string]string{"creatorId": "writer", "tag": "fanart"},
			invoke: func(c *fanbox.Client) error {
				page, err := c.TaggedPosts(context.Background(), fanbox.TaggedPostsRequest{CreatorID: "writer", Tag: "fanart"})
				if err == nil && (len(page.Items) != 1 || page.Items[0].ID != "tagged") {
					t.Errorf("page = %+v", page)
				}
				return err
			},
		},
		{
			name: "supporting", body: `{"body":{"items":[{"id":"support-1","title":"support","publishedDatetime":"2024-01-01T00:00:00Z"}]}}`,
			path: "/post.listSupporting", query: map[string]string{"limit": "10"},
			invoke: func(c *fanbox.Client) error {
				page, err := c.Supporting(context.Background(), fanbox.SupportingRequest{})
				if err == nil && (len(page.Items) != 1 || page.Items[0].ID != "support-1") {
					t.Errorf("page = %+v", page)
				}
				return err
			},
		},
		{
			name: "home", body: `{"body":{"items":[{"id":"home-1","title":"home","publishedDatetime":"2024-01-01T00:00:00Z"}],"nextUrl":"https://api.fanbox.cc/post.listHome?cursor=2"}}`,
			path: "/post.listHome", query: map[string]string{"limit": "10"},
			invoke: func(c *fanbox.Client) error {
				page, err := c.Home(context.Background(), fanbox.HomeRequest{})
				if err == nil && (len(page.Items) != 1 || page.Items[0].ID != "home-1" || page.Next.IsZero()) {
					t.Errorf("page = %+v", page)
				}
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, recorder := recorderClient(t, test.body)
			if err := test.invoke(client); err != nil {
				t.Fatalf("invoke: %v", err)
			}
			request := recorder.lastAPI(t)
			if request.Path != test.path {
				t.Fatalf("path = %q, want %q", request.Path, test.path)
			}
			for key, want := range test.query {
				if got := request.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestPostBodyVariants(t *testing.T) {
	const image = `"img-1":{"id":"img-1","originalUrl":"https://i.pximg.net/1.png"}`
	const file = `"file-1":{"id":"file-1","name":"clip","extension":"mp4","url":"https://downloads.fanbox.cc/1.mp4"}`
	tests := []struct {
		name     string
		body     string
		wantIDs  []string
		wantKind []fanbox.PostBlockKind
	}{
		{name: "images list", body: `{"images":[{"id":"img-1","originalUrl":"https://i.pximg.net/1.png"},{"id":"img-2","originalUrl":"https://i.pximg.net/2.png"}]}`, wantIDs: []string{"img-1", "img-2"}},
		{name: "files list", body: `{"files":[{"id":"file-1","name":"clip","extension":"mp4","url":"https://downloads.fanbox.cc/1.mp4"}]}`, wantIDs: []string{"file-1"}},
		{name: "repeated block reference", body: `{"blocks":[{"type":"image","imageId":"img-1"},{"type":"file","fileId":"file-1"},{"type":"image","imageId":"img-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`, wantIDs: []string{"img-1", "file-1"}, wantKind: []fanbox.PostBlockKind{fanbox.PostBlockImage, fanbox.PostBlockFile, fanbox.PostBlockImage}},
		{name: "file block before image block", body: `{"blocks":[{"type":"file","fileId":"file-1"},{"type":"image","imageId":"img-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`, wantIDs: []string{"img-1", "file-1"}, wantKind: []fanbox.PostBlockKind{fanbox.PostBlockFile, fanbox.PostBlockImage}},
		{name: "both ids on one block", body: `{"blocks":[{"type":"image","imageId":"img-1","fileId":"file-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`, wantIDs: []string{"img-1", "file-1"}, wantKind: []fanbox.PostBlockKind{fanbox.PostBlockImage}},
		{name: "article block carrying an image id", body: `{"blocks":[{"type":"article","imageId":"img-1"}],"imageMap":{` + image + `}}`, wantIDs: []string{"img-1"}, wantKind: []fanbox.PostBlockKind{fanbox.PostBlockArticle}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := recorderClient(t, `{"body":{"post":{"id":"p","publishedDatetime":"2024-01-01T00:00:00Z","body":`+test.body+`}}}`)
			post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p"})
			if err != nil {
				t.Fatal(err)
			}
			if post.Body == nil {
				t.Fatal("post body is nil")
			}
			var gotIDs []string
			for _, asset := range post.Body.Assets {
				gotIDs = append(gotIDs, asset.ID)
			}
			if strings.Join(gotIDs, ",") != strings.Join(test.wantIDs, ",") {
				t.Fatalf("asset ids = %v, want %v", gotIDs, test.wantIDs)
			}
			var gotBlockKinds []fanbox.PostBlockKind
			for _, block := range post.Body.Blocks {
				gotBlockKinds = append(gotBlockKinds, block.Kind)
			}
			if len(gotBlockKinds) != len(test.wantKind) {
				t.Fatalf("block kinds = %v, want %v", gotBlockKinds, test.wantKind)
			}
			for index := range gotBlockKinds {
				if gotBlockKinds[index] != test.wantKind[index] {
					t.Fatalf("block kinds = %v, want %v", gotBlockKinds, test.wantKind)
				}
			}
		})
	}
}

func TestPostResponseErrorsStayClassified(t *testing.T) {
	for name, test := range map[string]struct {
		body   string
		reason sdk.Reason
	}{
		"missing id":       {body: `{"body":{"post":{"publishedDatetime":"2024-01-01T00:00:00Z"}}}`, reason: sdk.UpstreamError},
		"malformed time":   {body: `{"body":{"post":{"id":"p","publishedDatetime":"yesterday"}}}`, reason: sdk.MalformedUpstreamResponse},
		"missing media":    {body: `{"body":{"post":{"id":"post-bad","publishedDatetime":"2024-01-01T00:00:00Z","body":{"blocks":[{"type":"image","imageId":"missing"}],"imageMap":{}}}}}`, reason: sdk.UpstreamError},
		"missing envelope": {body: `{}`, reason: sdk.UpstreamError},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := recorderClient(t, test.body)
			if _, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p"}); sdk.ReasonOf(err) != test.reason {
				t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), test.reason, err)
			}
		})
	}
}
