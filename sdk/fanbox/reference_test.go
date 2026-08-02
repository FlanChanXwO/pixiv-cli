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
	cases := []struct {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := client.ResolveURL(context.Background(), fanbox.ResolveURLRequest{RawURL: tc.raw})
			if err != nil {
				t.Fatalf("ResolveURL(%q): %v", tc.raw, err)
			}
			if ref.Kind != tc.kind || ref.CreatorID != tc.creatorID || ref.PostID != tc.postID || ref.Tag != tc.tag {
				t.Fatalf("ref = %+v", ref)
			}
		})
	}
}

func TestResolveURLRejectsInvalid(t *testing.T) {
	client, _ := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: "session"})
	cases := []string{
		"",
		"https://example.com/@pixiv",
		"http://www.fanbox.cc/@pixiv",
		"https://user:pass@www.fanbox.cc/@pixiv",
		"https://www.fanbox.cc/",
		"https://www.fanbox.cc/@pixiv/unknown",
		"https://www.fanbox.cc/@pixiv/posts/tag",
		"https://sub.sub.fanbox.cc",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := client.ResolveURL(context.Background(), fanbox.ResolveURLRequest{RawURL: raw})
			if sdk.CodeOf(err) != sdk.CodeInvalidArgument {
				t.Fatalf("ResolveURL(%q): expected CodeInvalidArgument, got %v", raw, err)
			}
		})
	}
}
