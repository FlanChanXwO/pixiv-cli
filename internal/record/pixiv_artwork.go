package record

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Tag 作品和小说共用标签，缺少翻译时省略 translated_name。
type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name,omitempty"`
}

// ImageResource 交付图片的稳定资源引用、尺寸与角色。
type ImageResource struct {
	Resource Resource `json:"resource"`
	Variant  string   `json:"variant"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
}

// Stamp 交付评论贴纸 ID 与图片引用。
type Stamp struct {
	ID    int64         `json:"id"`
	Image ImageResource `json:"image"`
}

// ArtworkPage 保留作品页序；PageIndex 从零开始。
type ArtworkPage struct {
	PageIndex int           `json:"page_index"`
	Image     ImageResource `json:"image"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
}

// ArtworkSeriesSummary 交付作品所属系列的 ID 与标题。
type ArtworkSeriesSummary struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Artwork 保存作品输出快照；列表省略 Pages，详情按上游页序填充。
type Artwork struct {
	ID                    int64                 `json:"id"`
	Title                 string                `json:"title"`
	Caption               string                `json:"caption"`
	ContentType           pixiv.ContentType     `json:"content_type"`
	Tags                  []Tag                 `json:"tags"`
	User                  User                  `json:"user"`
	PublishedAt           time.Time             `json:"published_at"`
	UpdatedAt             *time.Time            `json:"updated_at,omitempty"`
	TotalBookmarks        int                   `json:"total_bookmarks"`
	TotalViews            int                   `json:"total_views"`
	Width                 int                   `json:"width"`
	Height                int                   `json:"height"`
	PageCount             int                   `json:"page_count"`
	XRestrict             int                   `json:"x_restrict"`
	AIType                int                   `json:"ai_type"`
	Tools                 []string              `json:"tools,omitempty"`
	Cover                 ImageResource         `json:"cover,omitzero"`
	Pages                 []ArtworkPage         `json:"pages,omitempty"`
	IsBookmarked          *bool                 `json:"is_bookmarked,omitempty"`
	IsMuted               *bool                 `json:"is_muted,omitempty"`
	Visible               *bool                 `json:"visible,omitempty"`
	SanityLevel           *int                  `json:"sanity_level,omitempty"`
	RestrictionAttributes *[]string             `json:"restriction_attributes,omitempty"`
	Series                *ArtworkSeriesSummary `json:"series,omitempty"`
	TotalComments         *int                  `json:"total_comments,omitempty"`
}

// IsZero 报告图片是否缺少稳定资源引用。
func (image ImageResource) IsZero() bool { return image.Resource.Ref.IsZero() }
