package protocol

import (
	"net/url"
	"strconv"
)

const (
	AppSearchIllust        = "/v1/search/illust"
	AppSearchNovel         = "/v1/search/novel"
	AppSearchIllustOptions = "/v1/search/options"
	AppIllustDetail        = "/v1/illust/detail"
	AppIllustRelated       = "/v2/illust/related"
	AppIllustRanking       = "/v1/illust/ranking"
	AppSearchUser          = "/v1/search/user"
	AppUserDetail          = "/v1/user/detail"
	AppIllustRecommended   = "/v1/illust/recommended"
	AppIllustNew           = "/v1/illust/new"
	AppNovelRecommended    = "/v1/novel/recommended"
	AppNovelNew            = "/v1/novel/new"
	AppNovelFollow         = "/v1/novel/follow"
	AppUserRecommended     = "/v1/user/recommended"
	AppTrendingTagsIllust  = "/v1/trending-tags/illust"
	AppIllustFollow        = "/v2/illust/follow"
	AppIllustMyPixiv       = "/v2/illust/mypixiv"
	AppNovelMyPixiv        = "/v1/novel/mypixiv"
	AppUserIllusts         = "/v1/user/illusts"
	AppUserNovels          = "/v1/user/novels"
	AppUserBookmarks       = "/v1/user/bookmarks/illust"
	AppUserFollowing       = "/v1/user/following"
	AppUserMyPixiv         = "/v1/user/mypixiv"
	AppUgoiraMetadata      = "/v1/ugoira/metadata"
	AppBookmarkAdd         = "/v2/illust/bookmark/add"
	AppBookmarkDelete      = "/v1/illust/bookmark/delete"
	AppFollowAdd           = "/v1/user/follow/add"
	AppFollowDelete        = "/v1/user/follow/delete"
	OAuthToken             = "/auth/token"
	AppLogin               = "/web/v1/login"
	AppOAuthStart          = "/web/v1/users/auth/pixiv/start"
	WebRanking             = "/ranking.php"
)

func WebSearchArtworks(word string) string { return "/ajax/search/artworks/" + url.PathEscape(word) }
func WebIllustDetail(id int64) string      { return "/ajax/illust/" + strconv.FormatInt(id, 10) }
func WebIllustPages(id int64) string       { return WebIllustDetail(id) + "/pages" }
func WebUgoiraMetadata(id int64) string    { return WebIllustDetail(id) + "/ugoira_meta" }
