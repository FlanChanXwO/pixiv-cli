package fanbox_test

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestResolveURL(t *testing.T) {
	client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: "session"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, test := range []struct {
		name      string
		raw       string
		kind      fanbox.ReferenceKind
		creatorID string
		postID    string
		tag       string
	}{
		{name: "creator", raw: "https://www.fanbox.cc/@pixiv", kind: fanbox.ReferenceKindCreator, creatorID: "pixiv"},
		{name: "creator posts", raw: "https://www.fanbox.cc/@pixiv/posts", kind: fanbox.ReferenceKindCreatorPosts, creatorID: "pixiv"},
		{name: "post", raw: "https://www.fanbox.cc/@pixiv/posts/12345", kind: fanbox.ReferenceKindPost, creatorID: "pixiv", postID: "12345"},
		{name: "tag", raw: "https://www.fanbox.cc/@pixiv/posts/tag/illust", kind: fanbox.ReferenceKindTag, creatorID: "pixiv", tag: "illust"},
		{name: "subdomain", raw: "https://pixiv.fanbox.cc", kind: fanbox.ReferenceKindCreator, creatorID: "pixiv"},
		{name: "legacy creators", raw: "https://www.fanbox.cc/creators/pixiv", kind: fanbox.ReferenceKindCreator, creatorID: "pixiv"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref, err := client.ResolveURL(context.Background(), fanbox.ResolveURLRequest{RawURL: test.raw})
			if err != nil || ref.Kind != test.kind || ref.CreatorID != test.creatorID || ref.PostID != test.postID || ref.Tag != test.tag {
				t.Fatalf("ref = %+v err=%v", ref, err)
			}
		})
	}
	for _, raw := range []string{
		"",
		"https://example.com/@pixiv",
		"http://www.fanbox.cc/@pixiv",
		"https://user:pass@www.fanbox.cc/@pixiv",
		"https://www.fanbox.cc/",
		"https://www.fanbox.cc/@pixiv/unknown",
		"https://www.fanbox.cc/@pixiv/posts/tag",
		"https://sub.sub.fanbox.cc",
	} {
		t.Run("invalid/"+raw, func(t *testing.T) {
			_, err := client.ResolveURL(context.Background(), fanbox.ResolveURLRequest{RawURL: raw})
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ResolveURL(%q) = %v, want InvalidArgument", raw, err)
			}
		})
	}
}
