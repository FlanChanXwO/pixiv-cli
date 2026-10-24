package novel

// UserSummary 是小说响应中随作品返回的最小用户身份；它不拥有用户详情
// 或网络行为，避免 novel parent 反向依赖 user endpoint family。
type UserSummary struct {
	ID               int64
	Name             string
	Account          string
	Comment          string
	IsFollowed       bool
	ProfileImageURLs ProfileImageURLs
}

type ProfileImageURLs struct {
	Medium *string
}

type Tag struct {
	Name           string
	TranslatedName string
}

type ImageURLs struct {
	SquareMedium string
	Medium       string
	Large        string
	Original     string
}

// Novel 是 App API novel endpoint 共享的 normalized entity。
// 可选 wire 字段在 endpoint family 中校验并归一化为其 public 零值语义。
type Novel struct {
	ID             int64
	Title          string
	Caption        string
	XRestrict      int
	TextLength     int
	IsOriginal     bool
	User           UserSummary
	Tags           []Tag
	ImageURLs      ImageURLs
	CreateDate     string
	TotalBookmarks int
	TotalView      int
}
