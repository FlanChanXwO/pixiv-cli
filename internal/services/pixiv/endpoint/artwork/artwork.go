package artwork

// Tag 是 normalized artwork 中的 Pixiv tag，不携带 App API wire metadata。
type Tag struct {
	Name           string
	TranslatedName string
}

// UserSummary 是 artwork endpoint 返回的作者摘要。完整 profile 属于 user
// endpoint family；这里仅保留 artwork 读取所需的稳定字段。
type UserSummary struct {
	ID               int64
	Name             string
	Account          string
	Comment          string
	IsFollowed       bool
	ProfileImageURLs ProfileImageURLs
}

// ProfileImageURLs 是作者头像的 normalized locator 集合。
type ProfileImageURLs struct {
	Medium *string
}

// ImageURLs 是 artwork endpoint 解析后的媒体 locator 集合。它只存在于
// internal service entity，public SDK 会在边界处转换为 sdk.Resource。
type ImageURLs struct {
	SquareMedium string
	Medium       string
	Large        string
	Original     string
}

// SinglePage 是单页 artwork 的原图 locator。
type SinglePage struct {
	OriginalImageURL string
}

// MetaPage 是多页 artwork 的一页及其媒体 locator。
type MetaPage struct {
	PageIndex int
	Width     int
	Height    int
	Extension string
	ImageURLs ImageURLs
}

// Artwork 是跨 App artwork endpoint 归一化后的实体。分页 continuation
// 不属于实体，由各 endpoint family 的 Result 类型表达。
type Artwork struct {
	ID             int64
	Title          string
	Caption        string
	Type           string
	PageCount      int
	TotalBookmarks int
	TotalView      int
	XRestrict      int
	User           UserSummary
	Tags           []Tag
	ImageURLs      ImageURLs
	MetaSinglePage SinglePage
	MetaPages      []MetaPage
	AIType         int
	CreateDate     string
	Width          int
	Height         int
	Tools          []string
}

// UgoiraMetadata 是 ugoira detail family 的规范化播放元数据。
type UgoiraMetadata struct {
	ZipURLs UgoiraZipURLs
	Frames  []UgoiraFrame
}

type UgoiraZipURLs struct {
	Medium   string
	Original string
}

type UgoiraFrame struct {
	File  string
	Delay int
}

type TrendingTag struct {
	Tag            string
	TranslatedName string
	Artwork        Artwork
}

type Comment struct {
	ID            int64
	User          UserSummary
	Comment       string
	CreateDate    string
	ParentComment *Comment
}

type CommentAccessControl struct {
	CanComment bool
	IsLocked   bool
}

type BookmarkDetail struct {
	Restrict string
	Tags     []string
}

type BookmarkTag struct {
	Name  string
	Count int
}
