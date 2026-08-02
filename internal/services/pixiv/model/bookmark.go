package model

// IllustBookmarkDetail 是当前账号对单个作品的收藏详情。Restrict 为空表示未收藏。
type IllustBookmarkDetail struct {
	Restrict string   `json:"restrict"`
	Tags     []string `json:"tags"`
}

// BookmarkTag 是用户作品收藏中使用的一个标签。Count 是携带该标签的收藏数。
type BookmarkTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BookmarkTagList 是用户作品收藏标签的一个批次。
type BookmarkTagList struct {
	Tags               []BookmarkTag `json:"bookmark_tags"`
	NextOffset         int           `json:"-"`
	ContinuationExists bool          `json:"-"`
}
