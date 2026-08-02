package pixiv_test

import (
	"reflect"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// parityMapping records the PixivPy AppPixivAPI baseline methods and how v1
// covers them. "direct" and "equivalent" entries must exist as methods on
// *Client; "excluded" entries must not.
var parityMapping = []struct {
	baseline string
	v1       string
	status   string
}{
	// Reads.
	{"user_detail", "User", "direct"},
	{"user_illusts", "UserArtworks", "direct"},
	{"user_bookmarks_illust", "UserArtworkBookmarks", "direct"},
	{"user_bookmarks_novel", "UserNovelBookmarks", "direct"},
	{"user_related", "RelatedUsers", "direct"},
	{"user_recommended", "RecommendedUsers", "direct"},
	{"user_following", "UserFollowing", "direct"},
	{"user_follower", "UserFollowers", "direct"},
	{"user_mypixiv", "MyPixivUsers", "direct"},
	{"user_list", "UserBlockedUsers", "equivalent"},
	{"illust_follow", "FollowingArtworks", "direct"},
	{"illust_detail", "Artwork", "direct"},
	{"illust_comments", "ArtworkComments", "direct"},
	{"illust_related", "RelatedArtworks", "direct"},
	{"illust_recommended", "RecommendedArtworks", "direct"},
	{"illust_ranking", "ArtworkRanking", "direct"},
	{"trending_tags_illust", "TrendingArtworkTags", "direct"},
	{"search_illust", "SearchArtworks", "direct"},
	{"illust_bookmark_detail", "ArtworkBookmark", "direct"},
	{"user_bookmark_tags_illust", "UserArtworkBookmarkTags", "direct"},
	{"ugoira_metadata", "UgoiraMetadata", "equivalent"},
	{"illust_new", "LatestArtworks", "direct"},
	{"search_novel", "SearchNovels", "direct"},
	{"novel_detail", "Novel", "direct"},
	{"novel_series", "NovelSeries", "equivalent"},
	{"novel_comments", "NovelComments", "direct"},
	{"novel_recommended", "RecommendedNovels", "direct"},
	{"novel_new", "LatestNovels", "direct"},
	{"novel_follow", "FollowingNovels", "direct"},
	{"user_novels", "UserNovels", "direct"},
	{"webview_novel", "NovelContent", "equivalent"},

	// Mutations and connection.
	{"illust_bookmark_add", "AddBookmark", "equivalent"},
	{"illust_bookmark_delete", "RemoveBookmark", "direct"},
	{"user_follow_add", "FollowUser", "direct"},
	{"user_follow_delete", "UnfollowUser", "direct"},
	{"user_edit_ai_show_settings", "SetAIArtworkVisibility", "equivalent"},
	{"auth/refresh", "Open", "equivalent"},
	{"set_auth", "New", "equivalent"},
	{"download", "OpenResource", "equivalent"},

	// Excluded baseline interfaces.
	{"showcase_article", "ShowcaseArticle", "excluded"},
	{"novel_text", "NovelText", "excluded"},
	{"raw webview_novel HTML", "NovelRawContent", "excluded"},
	{"req_auth=false", "AnonymousSearchArtworks", "excluded"},
	{"username/password grant", "LoginWithPassword", "excluded"},
	{"parse_result", "ParseResult", "excluded"},
	{"set_api_proxy", "SetAPIProxy", "excluded"},
}

func TestPixivPyParityInventory(t *testing.T) {
	clientType := reflect.TypeOf((*pixiv.Client)(nil))
	packageFns := map[string]any{
		"Open": pixiv.Open,
		"New":  pixiv.New,
	}
	for _, entry := range parityMapping {
		if fn, ok := packageFns[entry.v1]; ok {
			if fn == nil {
				t.Errorf("%s -> %s (%s): function must exist", entry.baseline, entry.v1, entry.status)
			}
			continue
		}
		_, exists := clientType.MethodByName(entry.v1)
		switch entry.status {
		case "direct", "equivalent":
			if !exists {
				t.Errorf("%s -> %s (%s): method %s must exist on *Client", entry.baseline, entry.v1, entry.status, entry.v1)
			}
		case "excluded":
			if exists {
				t.Errorf("%s -> %s (excluded): method %s must NOT exist on *Client", entry.baseline, entry.v1, entry.v1)
			}
		default:
			t.Fatalf("unknown status %q for %s", entry.status, entry.baseline)
		}
	}
}

// TestPublicInventoryNoRawMediaURLFields guards the invariant that media URLs
// never appear as loose public fields; they may only live inside Resource.URL.
func TestPublicInventoryNoRawMediaURLFields(t *testing.T) {
	forbidden := []string{"DownloadURL", "ZipURLs", "OriginalURL", "SignedURL", "ImageURLs", "MetaSinglePage", "MetaPages"}
	clientType := reflect.TypeOf((*pixiv.Client)(nil))
	for i := 0; i < clientType.NumMethod(); i++ {
		method := clientType.Method(i)
		found := scanTypeForFields(method.Type, forbidden)
		if len(found) > 0 {
			t.Errorf("method %s exposes forbidden media URL fields: %v", method.Name, found)
		}
	}
}

func scanTypeForFields(typ reflect.Type, forbidden []string) []string {
	if typ.Kind() == reflect.Pointer {
		return scanTypeForFields(typ.Elem(), forbidden)
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	var found []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		for _, name := range forbidden {
			if field.Name == name {
				found = append(found, field.Name)
			}
		}
		if field.Type.Kind() == reflect.Struct || field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			if next := scanTypeForFields(field.Type, forbidden); len(next) > 0 {
				found = append(found, next...)
			}
		}
	}
	return found
}
