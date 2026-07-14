package protocol

import (
	"net/url"
	"strconv"
)

const (
	AppSearchIllust       = "/v1/search/illust"
	AppIllustDetail       = "/v1/illust/detail"
	AppIllustRelated      = "/v2/illust/related"
	AppIllustRanking      = "/v1/illust/ranking"
	AppSearchUser         = "/v1/search/user"
	AppUserDetail         = "/v1/user/detail"
	AppIllustRecommended  = "/v1/illust/recommended"
	AppNovelRecommended   = "/v1/novel/recommended"
	AppUserRecommended    = "/v1/user/recommended"
	AppTrendingTagsIllust = "/v1/trending-tags/illust"
	AppIllustFollow       = "/v2/illust/follow"
	AppUserIllusts        = "/v1/user/illusts"
	AppUserBookmarks      = "/v1/user/bookmarks/illust"
	AppUserFollowing      = "/v1/user/following"
	AppUgoiraMetadata     = "/v1/ugoira/metadata"
	AppBookmarkAdd        = "/v2/illust/bookmark/add"
	AppBookmarkDelete     = "/v1/illust/bookmark/delete"
	AppFollowAdd          = "/v1/user/follow/add"
	AppFollowDelete       = "/v1/user/follow/delete"
	OAuthToken            = "/auth/token"
	AppLogin              = "/web/v1/login"
	AppOAuthStart         = "/web/v1/users/auth/pixiv/start"
	WebRanking            = "/ranking.php"
)

func WebSearchArtworks(word string) string { return "/ajax/search/artworks/" + url.PathEscape(word) }
func WebIllustDetail(id int64) string      { return "/ajax/illust/" + strconv.FormatInt(id, 10) }
func WebIllustPages(id int64) string       { return WebIllustDetail(id) + "/pages" }
func WebUgoiraMetadata(id int64) string    { return WebIllustDetail(id) + "/ugoira_meta" }
