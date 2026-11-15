package record

import "time"

// Novel 保存小说元数据；缺失的 UpdatedAt 输出为 null。
type Novel struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	User           User          `json:"user"`
	Tags           []Tag         `json:"tags"`
	PublishedAt    time.Time     `json:"published_at"`
	UpdatedAt      *time.Time    `json:"updated_at"`
	XRestrict      int           `json:"x_restrict"`
	TextLength     int           `json:"text_length"`
	IsOriginal     bool          `json:"is_original"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalViews     int           `json:"total_views"`
	Cover          ImageResource `json:"cover,omitzero"`
}

// NovelSeries 保存小说系列元数据，作者使用 user 字段。
type NovelSeries struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	User        User   `json:"user"`
	IsConcluded bool   `json:"is_concluded"`
}
