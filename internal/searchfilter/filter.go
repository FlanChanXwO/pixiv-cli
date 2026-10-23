package searchfilter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// Rating 是产品层的本地 rating 语义。它不是 Pixiv App API 的请求参数。
type Rating string

// Rating 的 canonical 值。
const (
	RatingAll    Rating = "all"
	RatingSFW    Rating = "sfw"
	RatingR18    Rating = "r18"
	RatingR18G   Rating = "r18g"
	RatingMature Rating = "mature"
)

// ContentType 是 artwork 结果的本地 subtype/selector 语义。
type ContentType string

// ContentType 的 canonical 值。
const (
	ContentTypeAll             ContentType = "all"
	ContentTypeIllustAndUgoira ContentType = "illust-and-ugoira"
	ContentTypeIllust          ContentType = "illust"
	ContentTypeManga           ContentType = "manga"
	ContentTypeUgoira          ContentType = "ugoira"
)

// Filter 保存已规范化的本地筛选条件。零值等价于没有筛选。
// Rating 只用于 client-side matching，不能映射为 x_restrict 请求字段。
type Filter struct {
	Rating      Rating
	ContentType ContentType
}

// NormalizeFilter 将命令层输入归一化为共享 canonical 语义。
// 空值和显式 all 等价；illustration 是既有 public artwork kind 的兼容别名。
func NormalizeFilter(rating, contentType string) (Filter, error) {
	normalizedRating, err := NormalizeRating(rating)
	if err != nil {
		return Filter{}, err
	}
	normalizedContentType, err := NormalizeContentType(contentType)
	if err != nil {
		return Filter{}, err
	}
	return Filter{Rating: normalizedRating, ContentType: normalizedContentType}, nil
}

// NormalizeRating 将 rating 输入规范化为本地 canonical 值。
// 该值只用于结果匹配和 cursor binding，不对应任何 upstream query 参数。
func NormalizeRating(value string) (Rating, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(RatingAll):
		return RatingAll, nil
	case string(RatingSFW):
		return RatingSFW, nil
	case string(RatingR18):
		return RatingR18, nil
	case string(RatingR18G):
		return RatingR18G, nil
	case string(RatingMature):
		return RatingMature, nil
	default:
		return "", errors.New("rating must be one of sfw, r18, r18g, mature, all")
	}
}

// NormalizeContentType 将 artwork subtype/selector 规范化为 canonical 值。
// illustration 是既有 public artwork kind 的兼容别名，canonical 值为 illust。
func NormalizeContentType(value string) (ContentType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ContentTypeAll):
		return ContentTypeAll, nil
	case "illustration", string(ContentTypeIllust):
		return ContentTypeIllust, nil
	case string(ContentTypeIllustAndUgoira):
		return ContentTypeIllustAndUgoira, nil
	case string(ContentTypeManga):
		return ContentTypeManga, nil
	case string(ContentTypeUgoira):
		return ContentTypeUgoira, nil
	default:
		return "", errors.New("content-type must be one of all, illust-and-ugoira, illust, manga, ugoira")
	}
}

// FilterContext 生成只包含 canonical 本地语义的 opaque cursor context。
// context 只参与 SDK cursor binding，不会被当作 upstream 请求参数。
func FilterContext(rating, contentType string) (string, error) {
	filter, err := NormalizeFilter(rating, contentType)
	if err != nil {
		return "", err
	}
	return filter.CursorContext(), nil
}

// CursorContext 返回当前本地筛选语义的稳定摘要，不暴露原始输入。
func (f Filter) CursorContext() string {
	rating := f.Rating
	if rating == "" {
		rating = RatingAll
	}
	contentType := f.ContentType
	if contentType == "" {
		contentType = ContentTypeAll
	}
	sum := sha256.Sum256([]byte("filter/v1\n" + string(rating) + "\n" + string(contentType)))
	return hex.EncodeToString(sum[:])
}

// Matches 按 DTO 中的 x_restrict 与 artwork kind 执行本地筛选。
// x_restrict 的 0/1/2 分别对应 safe/r18/r18g；未知值只会在显式 rating
// 筛选时被排除，避免把未知上游值误判为已知安全级别。
func (f Filter) Matches(xRestrict int, kind string) bool {
	if !matchesRating(f.Rating, xRestrict) {
		return false
	}
	return matchesContentType(f.ContentType, kind)
}

func matchesRating(rating Rating, xRestrict int) bool {
	switch rating {
	case "", RatingAll:
		return true
	case RatingSFW:
		return xRestrict == 0
	case RatingR18:
		return xRestrict == 1
	case RatingR18G:
		return xRestrict == 2
	case RatingMature:
		return xRestrict == 1 || xRestrict == 2
	default:
		return false
	}
}

func matchesContentType(contentType ContentType, kind string) bool {
	if contentType == "" || contentType == ContentTypeAll {
		return true
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || kind == "illustration" {
		kind = string(ContentTypeIllust)
	}
	switch contentType {
	case ContentTypeIllust:
		return kind == string(ContentTypeIllust)
	case ContentTypeIllustAndUgoira:
		return kind == string(ContentTypeIllust) || kind == string(ContentTypeUgoira)
	case ContentTypeManga, ContentTypeUgoira:
		return kind == string(contentType)
	default:
		return false
	}
}
