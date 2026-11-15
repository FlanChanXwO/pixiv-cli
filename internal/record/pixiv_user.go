package record

// User 保存用户摘要，头像仅交付稳定引用。
type User struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Account      string        `json:"account"`
	Comment      string        `json:"comment"`
	IsFollowed   bool          `json:"is_followed"`
	ProfileImage ImageResource `json:"profile_image,omitzero"`
}

// UserDetail 保留用户详情的 profile/profile_publicity/workspace 布局。
type UserDetail struct {
	User             User                 `json:"user"`
	Profile          UserProfile          `json:"profile"`
	ProfilePublicity UserProfilePublicity `json:"profile_publicity"`
	Workspace        UserWorkspace        `json:"workspace"`
}

// UserProfile 保存用户公开资料的计数与链接。
type UserProfile struct {
	Webpage                   string `json:"webpage"`
	Gender                    string `json:"gender"`
	BirthDay                  string `json:"birth_day"`
	BirthYear                 int    `json:"birth_year"`
	Region                    string `json:"region"`
	CountryCode               string `json:"country_code"`
	Job                       string `json:"job"`
	TotalFollowUsers          int    `json:"total_follow_users"`
	TotalMyPixivUsers         int    `json:"total_my_pixiv_users"`
	TotalIllusts              int    `json:"total_illusts"`
	TotalManga                int    `json:"total_manga"`
	TotalNovels               int    `json:"total_novels"`
	TotalIllustBookmarks      int    `json:"total_illust_bookmarks"`
	TotalIllustSeries         int    `json:"total_illust_series"`
	TotalNovelSeries          int    `json:"total_novel_series"`
	BackgroundImageURL        string `json:"background_image_url"`
	TwitterAccount            string `json:"twitter_account"`
	TwitterURL                string `json:"twitter_url"`
	PawooURL                  string `json:"pawoo_url"`
	IsPremium                 bool   `json:"is_premium"`
	IsUsingCustomProfileImage bool   `json:"is_using_custom_profile_image"`
}

// UserProfilePublicity 记录用户资料字段的公开状态。
type UserProfilePublicity struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

// UserWorkspace 保存用户声明的工作环境与工具。
type UserWorkspace struct {
	PC                string `json:"pc"`
	Monitor           string `json:"monitor"`
	Tool              string `json:"tool"`
	Scanner           string `json:"scanner"`
	Tablet            string `json:"tablet"`
	Mouse             string `json:"mouse"`
	Printer           string `json:"printer"`
	Desktop           string `json:"desktop"`
	Music             string `json:"music"`
	Desk              string `json:"desk"`
	Chair             string `json:"chair"`
	Comment           string `json:"comment"`
	WorkspaceImageURL string `json:"workspace_image_url"`
}

// UserPreview 保存用户及样例作品；列表投影保留空的 illusts/novels 数组。
type UserPreview struct {
	User    User      `json:"user"`
	Illusts []Artwork `json:"illusts"`
	Novels  []Novel   `json:"novels"`
}
