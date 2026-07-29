package webapi

// 以下 DTO 只表达 Pixiv Web endpoint 的 wire shape；normalized model 的所有权仍在 internal/services/pixiv/model。
type ajaxEnvelope[T any] struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Body    T      `json:"body"`

	bodyPresent bool
}

type webSearchBody struct {
	IllustManga webSearchGroup `json:"illustManga"`
	Illust      webSearchGroup `json:"illust"`
}

type webSearchGroup struct {
	Data  requiredWebList[webSearchIllust] `json:"data"`
	Total flexInt64                        `json:"total"`
}

type requiredWebList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

type webSearchIllust struct {
	ID            flexInt64 `json:"id"`
	Title         string    `json:"title"`
	IllustType    flexInt   `json:"illustType"`
	XRestrict     flexInt   `json:"xRestrict"`
	AIType        flexInt   `json:"aiType"`
	URL           string    `json:"url"`
	Tags          []string  `json:"tags"`
	UserID        flexInt64 `json:"userId"`
	UserName      string    `json:"userName"`
	PageCount     flexInt   `json:"pageCount"`
	ProfileImgURL string    `json:"profileImageUrl"`
}

type webIllustDetail struct {
	ID            flexInt64     `json:"id"`
	IllustID      flexInt64     `json:"illustId"`
	Title         string        `json:"title"`
	IllustTitle   string        `json:"illustTitle"`
	Description   string        `json:"description"`
	IllustType    flexInt       `json:"illustType"`
	XRestrict     flexInt       `json:"xRestrict"`
	PageCount     flexInt       `json:"pageCount"`
	UserID        flexInt64     `json:"userId"`
	UserName      string        `json:"userName"`
	BookmarkCount flexInt       `json:"bookmarkCount"`
	ViewCount     flexInt       `json:"viewCount"`
	AIType        flexInt       `json:"aiType"`
	CreateDate    string        `json:"createDate"`
	Width         flexInt       `json:"width"`
	Height        flexInt       `json:"height"`
	Tags          webDetailTags `json:"tags"`
	URLs          webDetailURLs `json:"urls"`
}

type webDetailTags struct {
	Tags []webTag `json:"tags"`
}

type webTag struct {
	Tag         string `json:"tag"`
	Translation struct {
		En string `json:"en"`
	} `json:"translation"`
}

type webDetailURLs struct {
	Mini      string `json:"mini"`
	ThumbMini string `json:"thumb_mini"`
	Small     string `json:"small"`
	Regular   string `json:"regular"`
	Original  string `json:"original"`
}

type webPage struct {
	URLs   webPageURLs `json:"urls"`
	Width  flexInt     `json:"width"`
	Height flexInt     `json:"height"`
}

type webPageURLs struct {
	ThumbMini string `json:"thumb_mini"`
	Small     string `json:"small"`
	Regular   string `json:"regular"`
	Original  string `json:"original"`
}

type webRankingResponse struct {
	Contents  requiredWebList[webRankingItem] `json:"contents"`
	RankTotal flexInt64                       `json:"rank_total"`
}

type webRankingItem struct {
	Title           string    `json:"title"`
	Tags            []string  `json:"tags"`
	URL             string    `json:"url"`
	IllustType      flexInt   `json:"illust_type"`
	IllustPageCount flexInt   `json:"illust_page_count"`
	UserName        string    `json:"user_name"`
	IllustID        flexInt64 `json:"illust_id"`
	UserID          flexInt64 `json:"user_id"`
	RatingCount     flexInt   `json:"rating_count"`
	ViewCount       flexInt   `json:"view_count"`
}

type webUgoiraMeta struct {
	OriginalSrc string           `json:"originalSrc"`
	Src         string           `json:"src"`
	Frames      []webUgoiraFrame `json:"frames"`
}

type webUgoiraFrame struct {
	File  string  `json:"file"`
	Delay flexInt `json:"delay"`
}

type flexInt64 int64

type flexInt int
