// Endpoint-family route and response-mapping tests: each case pins the request
// path/query and the fields the old endpoint packages asserted.

package fanbox_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

type recordedRequest struct {
	url *url.URL
	raw string
}

// endpointRecorder serves one JSON body per call and records every request.
type endpointRecorder struct {
	bodies   []string
	requests []recordedRequest
}

func (r *endpointRecorder) roundTrip(req *http.Request) (*http.Response, error) {
	r.requests = append(r.requests, recordedRequest{url: req.URL, raw: req.URL.String()})
	if req.URL.Host == "www.fanbox.cc" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html"}},
			Body:       io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester"))),
		}, nil
	}
	if len(r.bodies) == 0 {
		return nil, errors.New("unexpected request " + req.URL.String())
	}
	body := r.bodies[0]
	r.bodies = r.bodies[1:]
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// lastAPI returns the most recent API request. Identity lookups hit the web
// host and can follow the request under assertion when cursors are built.
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

func TestCreatorProfileMapsFieldsAndRoute(t *testing.T) {
	client, recorder := recorderClient(t, `{"body":{"creatorId":"creator value","user":{"name":"Creator Name","iconUrl":"https://i.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://i.pximg.net/cover.png","plan":{"fee":500,"hasSupportingPlan":true}}}`)
	profile, err := client.Creator(context.Background(), fanbox.CreatorRequest{CreatorID: "creator value"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "creator value" || profile.Name != "Creator Name" || profile.PlanFee != 500 || !profile.HasSupportingPlan {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Icon.Resource.URL != "https://i.pximg.net/icon.png" || profile.Cover.Resource.URL != "https://i.pximg.net/cover.png" {
		t.Fatalf("profile media = %+v / %+v", profile.Icon, profile.Cover)
	}
	request := recorder.lastAPI(t)
	if request.Path != "/creator.get" || request.Query().Get("creatorId") != "creator value" {
		t.Fatalf("profile endpoint = %q", request.String())
	}
}

func TestCreatorTagsMapFeaturedTagsAndQuery(t *testing.T) {
	client, recorder := recorderClient(t, `{"body":{"tags":[{"tag":"fanart","url":"https://www.fanbox.cc/@writer/posts/tag/fanart"}]}}`)
	items, err := client.CreatorTags(context.Background(), fanbox.CreatorTagsRequest{CreatorID: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "fanart" || items[0].URL == "" {
		t.Fatalf("items = %+v", items)
	}
	request := recorder.lastAPI(t)
	if request.Path != "/tag.getFeatured" || request.Query().Get("creatorId") != "writer" {
		t.Fatalf("endpoint = %q", request.String())
	}
}

func TestTaggedPostsUseCreatorQuery(t *testing.T) {
	client, recorder := recorderClient(t, `{"body":{"posts":[{"id":"tagged","title":"tagged","publishedDatetime":"2024-01-01T00:00:00Z"}]}}`)
	page, err := client.TaggedPosts(context.Background(), fanbox.TaggedPostsRequest{CreatorID: "writer", Tag: "fanart"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "tagged" {
		t.Fatalf("page = %+v", page)
	}
	request := recorder.lastAPI(t)
	if request.Path != "/post.listTagged" || request.Query().Get("creatorId") != "writer" || request.Query().Get("tag") != "fanart" {
		t.Fatalf("tagged endpoint = %q", request.String())
	}
}

func TestSupportingUsesRouteAndItemsEnvelope(t *testing.T) {
	client, recorder := recorderClient(t, `{"body":{"items":[{"id":"support-1","title":"support","publishedDatetime":"2024-01-01T00:00:00Z"}]}}`)
	page, err := client.Supporting(context.Background(), fanbox.SupportingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "support-1" {
		t.Fatalf("page = %+v", page)
	}
	request := recorder.lastAPI(t)
	if request.Path != "/post.listSupporting" || request.Query().Get("limit") != "10" {
		t.Fatalf("endpoint = %q", request.String())
	}
}

func TestHomeDecodesItemsEnvelope(t *testing.T) {
	client, recorder := recorderClient(t, `{"body":{"items":[{"id":"home-1","title":"home","publishedDatetime":"2024-01-01T00:00:00Z"}],"nextUrl":"https://api.fanbox.cc/post.listHome?cursor=2"}}`, `{"body":{"items":[]}}`)
	page, err := client.Home(context.Background(), fanbox.HomeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "home-1" || page.Next.IsZero() {
		t.Fatalf("page = %+v", page)
	}
	request := recorder.lastAPI(t)
	if request.Path != "/post.listHome" || request.Query().Get("limit") != "10" {
		t.Fatalf("endpoint = %q", request.String())
	}
}

// TestPostRejectsMissingMediaReference keeps the previous wire assertion that a
// block pointing at absent imageMap media fails the operation.
func TestPostRejectsMissingMediaReference(t *testing.T) {
	client, _ := recorderClient(t, `{"body":{"post":{"id":"post-bad","publishedDatetime":"2024-01-01T00:00:00Z","body":{"blocks":[{"type":"image","imageId":"missing"}],"imageMap":{}}}}}`)
	if _, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "post-bad"}); err == nil {
		t.Fatal("missing media reference unexpectedly succeeded")
	}
}

// TestPostBodyVariants pins the asset semantics the previous SDK exposed:
// every image before every file, block media deduplicated by id, and an
// imageId/fileId collected even on a block whose type is not image/file.
func TestPostBodyVariants(t *testing.T) {
	const image = `"img-1":{"id":"img-1","originalUrl":"https://i.pximg.net/1.png"}`
	const file = `"file-1":{"id":"file-1","name":"clip","extension":"mp4","url":"https://downloads.fanbox.cc/1.mp4"}`
	tests := []struct {
		name     string
		body     string
		wantIDs  []string
		wantKind []fanbox.PostBlockKind
	}{
		{
			name:     "images list",
			body:     `{"images":[{"id":"img-1","originalUrl":"https://i.pximg.net/1.png"},{"id":"img-2","originalUrl":"https://i.pximg.net/2.png"}]}`,
			wantIDs:  []string{"img-1", "img-2"},
			wantKind: nil,
		},
		{
			name:     "files list",
			body:     `{"files":[{"id":"file-1","name":"clip","extension":"mp4","url":"https://downloads.fanbox.cc/1.mp4"}]}`,
			wantIDs:  []string{"file-1"},
			wantKind: nil,
		},
		{
			name:    "repeated block reference",
			body:    `{"blocks":[{"type":"image","imageId":"img-1"},{"type":"file","fileId":"file-1"},{"type":"image","imageId":"img-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`,
			wantIDs: []string{"img-1", "file-1"},
			wantKind: []fanbox.PostBlockKind{
				fanbox.PostBlockImage, fanbox.PostBlockFile, fanbox.PostBlockImage,
			},
		},
		{
			name:    "file block before image block",
			body:    `{"blocks":[{"type":"file","fileId":"file-1"},{"type":"image","imageId":"img-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`,
			wantIDs: []string{"img-1", "file-1"},
			wantKind: []fanbox.PostBlockKind{
				fanbox.PostBlockFile, fanbox.PostBlockImage,
			},
		},
		{
			name:    "both ids on one block",
			body:    `{"blocks":[{"type":"image","imageId":"img-1","fileId":"file-1"}],"imageMap":{` + image + `},"fileMap":{` + file + `}}`,
			wantIDs: []string{"img-1", "file-1"},
			wantKind: []fanbox.PostBlockKind{
				fanbox.PostBlockImage,
			},
		},
		{
			name:    "article block carrying an image id",
			body:    `{"blocks":[{"type":"article","imageId":"img-1"}],"imageMap":{` + image + `}}`,
			wantIDs: []string{"img-1"},
			wantKind: []fanbox.PostBlockKind{
				fanbox.PostBlockArticle,
			},
		},
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
			var gotKinds []fanbox.AssetKind
			for _, asset := range post.Body.Assets {
				gotIDs = append(gotIDs, asset.ID)
				gotKinds = append(gotKinds, asset.Kind)
			}
			if strings.Join(gotIDs, ",") != strings.Join(test.wantIDs, ",") {
				t.Fatalf("asset ids = %v, want %v", gotIDs, test.wantIDs)
			}
			if len(gotKinds) > 0 && gotKinds[0] == fanbox.AssetKindImage {
				for _, kind := range gotKinds {
					if kind != fanbox.AssetKindImage && kind != fanbox.AssetKindFile {
						t.Fatalf("unexpected asset kind %q", kind)
					}
				}
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

// TestPostResponseErrorsStayClassified keeps the previous boundary contract:
// wire failures surface as classified SDK errors, while a malformed publish
// time keeps its specific reason.
func TestPostResponseErrorsStayClassified(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		reason sdk.Reason
	}{
		{name: "missing id", body: `{"body":{"post":{"publishedDatetime":"2024-01-01T00:00:00Z"}}}`, reason: sdk.UpstreamError},
		{name: "malformed time", body: `{"body":{"post":{"id":"p","publishedDatetime":"yesterday"}}}`, reason: sdk.MalformedUpstreamResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := recorderClient(t, test.body)
			_, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p"})
			if got := sdk.ReasonOf(err); got != test.reason {
				t.Fatalf("reason = %q, want %q (err=%v)", got, test.reason, err)
			}
		})
	}
}
