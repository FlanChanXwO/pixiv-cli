package fanbox

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestCreatorDecodesProfileFields(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Host != "api.fanbox.cc" || request.URL.Path != "/creator.get" {
			t.Errorf("unexpected creator profile request %s host=%q", request.URL, request.Host)
		}
		if got := request.URL.Query().Get("creatorId"); got != "creator value" {
			t.Errorf("creatorId = %q", got)
		}
		if got := request.Header.Get("Cookie"); got != "FANBOXSESSID=creator-profile-canary" {
			t.Errorf("Cookie = %q", got)
		}
		writeJSON(w, `{"body":{"creatorId":"creator value","user":{"name":"Creator Name","iconUrl":"https://i.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://i.pximg.net/cover.png","plan":{"fee":500,"hasSupportingPlan":true}}}`)
	}), "FANBOXSESSID=creator-profile-canary")

	profile, err := session.Creator(context.Background(), "creator value")
	if err != nil {
		t.Fatal(err)
	}
	want := CreatorProfile{
		ID:                "creator value",
		DisplayName:       "Creator Name",
		IconURL:           "https://i.pximg.net/icon.png",
		HasAdultContent:   true,
		IsFollowing:       true,
		CoverURL:          "https://i.pximg.net/cover.png",
		PlanFee:           500,
		HasSupportingPlan: true,
	}
	if profile != want {
		t.Fatalf("Creator() = %+v, want %+v", profile, want)
	}
}

func TestCreatorRejectsMissingDisplayName(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, `{"body":{"creatorId":"creator value","user":{}}}`)
	}), "FANBOXSESSID=creator-minimal-canary")

	if _, err := session.Creator(context.Background(), "creator value"); err == nil {
		t.Fatal("Creator() unexpectedly accepted an empty display name")
	}
}

func TestCreatorPostsPaginationFetchesNextURLDirectly(t *testing.T) {
	nextURL := "https://api.fanbox.cc/post.listCreator?creatorId=writer&maxPublishedDatetime=abc"
	var seen []string
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen = append(seen, "https://"+request.Host+request.URL.RequestURI())
		if request.URL.Path != "/post.listCreator" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		if request.URL.Query().Get("maxPublishedDatetime") == "" {
			writeJSON(w, fmt.Sprintf(`{"body":{"posts":[{"id":"first","title":"first"}],"nextUrl":%q}}`, nextURL))
			return
		}
		writeJSON(w, `{"body":{"posts":[{"id":"second","title":"second"}],"nextUrl":""}}`)
	}), "FANBOXSESSID=posts-canary")

	page1, err := session.CreatorPosts(context.Background(), "writer", "")
	if err != nil {
		t.Fatalf("CreatorPosts() page1 error = %v", err)
	}
	if got, want := postIDs(page1.Posts), []string{"first"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("page1 post ids = %v, want %v", got, want)
	}
	if page1.NextURL != nextURL {
		t.Fatalf("page1 NextURL = %q, want %q", page1.NextURL, nextURL)
	}

	page2, err := session.CreatorPosts(context.Background(), "writer", page1.NextURL)
	if err != nil {
		t.Fatalf("CreatorPosts() page2 error = %v", err)
	}
	if got, want := postIDs(page2.Posts), []string{"second"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("page2 post ids = %v, want %v", got, want)
	}
	if len(seen) != 2 || seen[1] != nextURL {
		t.Fatalf("second request = %q, want exact nextURL %q (seen=%v)", seen[1], nextURL, seen)
	}
}

func TestTaggedPostsUsesCreatorAndTag(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/post.listTaggedPosts" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		if got := request.URL.Query().Get("creatorId"); got != "writer" {
			t.Errorf("creatorId = %q", got)
		}
		if got := request.URL.Query().Get("tag"); got != "fanart" {
			t.Errorf("tag = %q", got)
		}
		writeJSON(w, `{"body":{"posts":[{"id":"tagged-1","title":"tagged"}]}}`)
	}), "FANBOXSESSID=tagged-canary")

	page, err := session.TaggedPosts(context.Background(), "writer", "fanart", "")
	if err != nil {
		t.Fatalf("TaggedPosts() error = %v", err)
	}
	if got, want := postIDs(page.Posts), []string{"tagged-1"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("post ids = %v, want %v", got, want)
	}
}

func TestPostInfoBlocksToAssets(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/post.info" || request.URL.Query().Get("postId") != "post-1" {
			t.Errorf("unexpected request %q", request.URL)
		}
		writeJSON(w, `{"body":{"post":{"id":"post-1","title":"title","restrictedFor":2,"commentCount":3,"body":{"blocks":[{"type":"image","imageId":"image-2"},{"type":"file","fileId":"file-1"},{"type":"image","imageId":"image-1"}],"imageMap":{"image-1":{"id":"image-1","extension":"jpg","originalUrl":"https://downloads.fanbox.cc/image-1.jpg"},"image-2":{"id":"image-2","extension":"png","originalUrl":"https://downloads.fanbox.cc/image-2.png"}},"fileMap":{"file-1":{"id":"file-1","name":"file","extension":"zip","url":"https://downloads.fanbox.cc/file-1.zip"}}}}}}`)
	}), "FANBOXSESSID=assets-canary")

	post, err := session.Post(context.Background(), "post-1")
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if post.Body == nil {
		t.Fatal("Post() body is nil")
	}
	if got, want := assetIDs(post.Body.Assets), []string{"image-2", "file-1", "image-1"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("assets = %v, want %v", got, want)
	}
	if post.RestrictedFor != 2 || post.CommentCount != 3 {
		t.Fatalf("post = %+v", post)
	}
	if post.Body.Assets[0].Kind != AssetKindImage || post.Body.Assets[1].Kind != AssetKindFile {
		t.Fatalf("asset kinds = %+v", post.Body.Assets)
	}
}

func TestPostInfoStrictlyRejectsMissingImageReference(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		writeJSON(w, `{"body":{"post":{"id":"post-bad","body":{"blocks":[{"type":"image","imageId":"missing"}],"imageMap":{}}}}}`)
	}), "FANBOXSESSID=assets-strict-canary")

	if _, err := session.Post(context.Background(), "post-bad"); err == nil {
		t.Fatal("Post() unexpectedly accepted a missing image reference")
	}
}

func TestCreatorsSupportingAndFollowing(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Host != "api.fanbox.cc" || request.Header.Get("Cookie") != "FANBOXSESSID=creator-canary" {
			t.Errorf("unexpected creator request host=%q cookie=%q", request.Host, request.Header.Get("Cookie"))
		}
		switch request.URL.Path {
		case "/plan.listSupporting":
			writeJSON(w, `{"body":{"plans":[{"creatorId":"supported","fee":500,"perks":[]}]}}`)
		case "/creator.listFollowing":
			writeJSON(w, `{"body":{"creators":[{"creatorId":"followed"}]}}`)
		default:
			t.Errorf("unexpected endpoint %q", request.URL.Path)
			writeStatus(w, http.StatusNotFound, `{}`)
		}
	}), "FANBOXSESSID=creator-canary")

	for _, test := range []struct {
		kind CreatorListKind
		want string
	}{
		{CreatorListSupporting, "supported"},
		{CreatorListFollowing, "followed"},
	} {
		creators, err := session.Creators(context.Background(), test.kind)
		if err != nil {
			t.Fatalf("Creators(%s) error = %v", test.kind, err)
		}
		if len(creators) != 1 || creators[0].ID != test.want {
			t.Fatalf("Creators(%s) = %+v", test.kind, creators)
		}
	}
}

func TestHomeAndSupportingDecodeItemsEnvelope(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/home.posts":
			writeJSON(w, `{"body":{"items":[{"id":"home-1","title":"home"}],"nextUrl":"https://api.fanbox.cc/home.posts?cursor=2"}}`)
		case "/home.supporting":
			writeJSON(w, `{"body":{"items":[{"id":"support-1","title":"support"}]}}`)
		default:
			t.Errorf("unexpected endpoint %q", request.URL.Path)
			writeStatus(w, http.StatusNotFound, `{}`)
		}
	}), "FANBOXSESSID=home-canary")

	home, err := session.Home(context.Background(), "")
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if got, want := postIDs(home.Posts), []string{"home-1"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Home() ids = %v, want %v", got, want)
	}
	if home.NextURL != "https://api.fanbox.cc/home.posts?cursor=2" {
		t.Fatalf("Home() NextURL = %q", home.NextURL)
	}

	supporting, err := session.Supporting(context.Background(), "")
	if err != nil {
		t.Fatalf("Supporting() error = %v", err)
	}
	if got, want := postIDs(supporting.Posts), []string{"support-1"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Supporting() ids = %v, want %v", got, want)
	}
}

func postIDs(posts []Post) []string {
	result := make([]string, 0, len(posts))
	for _, post := range posts {
		result = append(result, post.ID)
	}
	return result
}

func assetIDs(assets []Asset) []string {
	result := make([]string, 0, len(assets))
	for _, asset := range assets {
		result = append(result, asset.ID)
	}
	return result
}
