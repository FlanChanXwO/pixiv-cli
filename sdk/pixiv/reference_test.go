package pixiv_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestParseURLCanonical(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		kind      pixiv.ReferenceKind
		id        int64
		owner     int64
		canonical string
	}{
		{name: "artwork", raw: "https://www.pixiv.net/artworks/123", kind: pixiv.ReferenceKindArtwork, id: 123, canonical: "https://www.pixiv.net/artworks/123"},
		{name: "artwork locale", raw: "https://www.pixiv.net/en/artworks/456", kind: pixiv.ReferenceKindArtwork, id: 456, canonical: "https://www.pixiv.net/artworks/456"},
		{name: "artwork host", raw: "https://pixiv.net/artworks/789", kind: pixiv.ReferenceKindArtwork, id: 789, canonical: "https://www.pixiv.net/artworks/789"},
		{name: "novel show php", raw: "https://www.pixiv.net/novel/show.php?id=1001", kind: pixiv.ReferenceKindNovel, id: 1001, canonical: "https://www.pixiv.net/novel/show.php?id=1001"},
		{name: "user", raw: "https://www.pixiv.net/users/2001", kind: pixiv.ReferenceKindUser, id: 2001, canonical: "https://www.pixiv.net/users/2001"},
		{name: "user artworks", raw: "https://www.pixiv.net/users/2001/artworks", kind: pixiv.ReferenceKindUser, id: 2001, canonical: "https://www.pixiv.net/users/2001"},
		{name: "user bookmarks", raw: "https://www.pixiv.net/users/2001/bookmarks/artworks", kind: pixiv.ReferenceKindUserBookmarks, id: 2001, canonical: "https://www.pixiv.net/users/2001/bookmarks/artworks"},
		{name: "artwork series", raw: "https://www.pixiv.net/user/3001/series/4001", kind: pixiv.ReferenceKindArtworkSeries, id: 4001, owner: 3001, canonical: "https://www.pixiv.net/user/3001/series/4001"},
		{name: "novel series", raw: "https://www.pixiv.net/novel/series/5001", kind: pixiv.ReferenceKindNovelSeries, id: 5001, canonical: "https://www.pixiv.net/novel/series/5001"},
		{name: "query stripped", raw: "https://www.pixiv.net/artworks/123?ref=tracking&p=1", kind: pixiv.ReferenceKindArtwork, id: 123, canonical: "https://www.pixiv.net/artworks/123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := pixiv.ParseURL(tc.raw)
			if err != nil {
				t.Fatalf("ParseURL(%q): %v", tc.raw, err)
			}
			if ref.Kind != tc.kind || ref.ID != tc.id || ref.OwnerUserID != tc.owner {
				t.Fatalf("ref = %+v, want kind=%q id=%d owner=%d", ref, tc.kind, tc.id, tc.owner)
			}
			canonical, err := ref.CanonicalURL()
			if err != nil {
				t.Fatalf("CanonicalURL: %v", err)
			}
			if canonical != tc.canonical {
				t.Fatalf("canonical = %q, want %q", canonical, tc.canonical)
			}
		})
	}
}

func TestParseURLRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"12345",
		"https://example.com/artworks/123",
		"http://www.pixiv.net/artworks/123",
		"https://www.pixiv.net/artworks/0",
		"https://www.pixiv.net/artworks/-5",
		"https://www.pixiv.net/artworks/abc",
		"https://www.pixiv.net/artworks/123/extra",
		"https://user:pass@www.pixiv.net/artworks/123",
		"https://www.pixiv.net:8080/artworks/123",
		"https://www.pixiv.net/novel/show.php?id=5&id=6",
		"https://www.pixiv.net/novel/show.php",
		"https://www.pixiv.net/unknown/path",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := pixiv.ParseURL(in)
			if err == nil {
				t.Fatalf("ParseURL(%q) should fail", in)
			}
		})
	}
}

func TestReferenceCanonicalURLRejectsZero(t *testing.T) {
	_, err := (pixiv.Reference{Kind: pixiv.ReferenceKindArtwork}).CanonicalURL()
	if err == nil {
		t.Fatal("zero ID reference should fail CanonicalURL")
	}
	_, err = (pixiv.Reference{Kind: pixiv.ReferenceKindArtworkSeries, ID: 5}).CanonicalURL()
	if err == nil {
		t.Fatal("artwork series without owner should fail CanonicalURL")
	}
}

func TestParseURLRejectsBareInteger(t *testing.T) {
	_, err := pixiv.ParseURL("12345")
	if err == nil {
		t.Fatal("bare integer must not be guessed to a resource type")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("error should be classified: %v", err)
	}
}
