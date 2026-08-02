package model

// NovelDetail 是小说详情，包含可选的前后系列引用。SeriesNextID/SeriesPrevID
// 为 0 表示该小说不属于任何系列。
type NovelDetail struct {
	Novel        Novel  `json:"novel"`
	SeriesNextID int64  `json:"series_next_id"`
	SeriesPrevID int64  `json:"series_prev_id"`
	SeriesTitle  string `json:"series_title,omitempty"`
}

// NovelSeries 是一个可寻址的小说系列。
type NovelSeries struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	User        User   `json:"user"`
	IsConcluded bool   `json:"is_concluded"`
}

// NovelSeriesResult 携带系列元数据及其分页小说批次。NextValue 记录 next_url
// 中的 last_order 续传值；ContinuationExists 为真时才有效。
type NovelSeriesResult struct {
	Series             NovelSeries `json:"novel_series_detail"`
	Novels             []Novel     `json:"novels"`
	NextValue          int64       `json:"-"`
	ContinuationExists bool        `json:"-"`
}
