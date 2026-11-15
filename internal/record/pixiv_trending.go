package record

// TrendingTag 保存趋势标签与样例作品。
type TrendingTag struct {
	Tag            string  `json:"tag"`
	TranslatedName string  `json:"translated_name"`
	Artwork        Artwork `json:"artwork"`
}
