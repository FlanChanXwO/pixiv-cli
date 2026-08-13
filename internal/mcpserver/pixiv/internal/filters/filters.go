// Package filters 持有 Pixiv MCP 实体 list tool 的本地筛选契约与实现。
package filters

import (
	"context"
	"errors"
	"fmt"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// IllustFilter 仅出现在 artwork list tool 的 schema 中。筛选在每个上游批次进入
// 逻辑分页前执行，避免把不匹配项计入 limit。
type IllustFilter struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive illustration ID"`
	Type     string   `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
	MinPages *int     `json:"min_pages,omitempty" jsonschema:"minimum page count"`
}

// NovelFilter 仅出现在 novel list tool 的 schema 中。
type NovelFilter struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive novel ID"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
}

// UserFilter 仅出现在 user list tool 的 schema 中。
type UserFilter struct {
	ID *int64 `json:"id,omitempty" jsonschema:"optional positive user ID"`
}

// IllustFilterSchema 固定 artwork 筛选字段，避免错误实体的字段在 MCP 协议层被
// 悄然接受。
func IllustFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":        map[string]any{"type": "integer", "minimum": 1},
			"type":      map[string]any{"type": "string", "enum": []string{"illust", "manga", "ugoira"}},
			"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"min_views": map[string]any{"type": "integer", "minimum": 0},
			"min_pages": map[string]any{"type": "integer", "minimum": 0},
		},
	}
}

// NovelFilterSchema 固定 novel 筛选字段。
func NovelFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":        map[string]any{"type": "integer", "minimum": 1},
			"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"min_views": map[string]any{"type": "integer", "minimum": 0},
		},
	}
}

type contextKey struct{}

type value struct {
	illust *IllustFilter
	novel  *NovelFilter
	user   *UserFilter
}

// WithIllustFilter 在打开 SDK client 前校验结构化筛选，非法输入不会产生网络请求。
func WithIllustFilter(ctx context.Context, filter *IllustFilter) (context.Context, error) {
	if err := ValidateIllustFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{illust: filter}), nil
}

// WithNovelFilter 校验 novel 筛选。
func WithNovelFilter(ctx context.Context, filter *NovelFilter) (context.Context, error) {
	if err := ValidateNovelFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{novel: filter}), nil
}

// WithUserFilter 校验 user 筛选。
func WithUserFilter(ctx context.Context, filter *UserFilter) (context.Context, error) {
	if err := ValidateUserFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{user: filter}), nil
}

// WithMixedFilters 校验 recommended 各实体的结构化筛选。
func WithMixedFilters(ctx context.Context, illust *IllustFilter, novel *NovelFilter, user *UserFilter) (context.Context, error) {
	if err := ValidateIllustFilter(illust); err != nil {
		return nil, err
	}
	if err := ValidateNovelFilter(novel); err != nil {
		return nil, err
	}
	if err := ValidateUserFilter(user); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{illust: illust, novel: novel, user: user}), nil
}

func withFilters(ctx context.Context, filters value) context.Context {
	return context.WithValue(ctx, contextKey{}, filters)
}

// FromContext 读取当前筛选。
func FromContext(ctx context.Context) (illust *IllustFilter, novel *NovelFilter, user *UserFilter) {
	filters, _ := ctx.Value(contextKey{}).(value)
	return filters.illust, filters.novel, filters.user
}

// ValidateIllustFilter 校验 artwork 筛选。
func ValidateIllustFilter(filter *IllustFilter) error {
	if filter == nil {
		return nil
	}
	if filter.ID != nil && *filter.ID <= 0 {
		return errors.New("illust_filter.id must be positive")
	}
	if filter.Type != "" && filter.Type != "illust" && filter.Type != "manga" && filter.Type != "ugoira" {
		return errors.New("illust_filter.type must be one of: illust, manga, ugoira")
	}
	if filter.MinViews != nil && *filter.MinViews < 0 {
		return errors.New("illust_filter.min_views must be zero or positive")
	}
	if filter.MinPages != nil && *filter.MinPages < 0 {
		return errors.New("illust_filter.min_pages must be zero or positive")
	}
	return nil
}

// ValidateNovelFilter 校验 novel 筛选。
func ValidateNovelFilter(filter *NovelFilter) error {
	if filter == nil {
		return nil
	}
	if filter.ID != nil && *filter.ID <= 0 {
		return errors.New("novel_filter.id must be positive")
	}
	if filter.MinViews != nil && *filter.MinViews < 0 {
		return errors.New("novel_filter.min_views must be zero or positive")
	}
	return nil
}

// ValidateUserFilter 校验 user 筛选。
func ValidateUserFilter(filter *UserFilter) error {
	if filter != nil && filter.ID != nil && *filter.ID <= 0 {
		return errors.New("user_filter.id must be positive")
	}
	return nil
}

// FilterPage 对单个上游批次执行本地筛选与去重；返回空时调用方不应计入 limit。
func FilterPage[T any](ctx context.Context, items []T, seen map[string]struct{}) []T {
	illust, novel, user := FromContext(ctx)
	filtered := make([]T, 0, len(items))
	for _, item := range items {
		key, matches := match(any(item), value{illust: illust, novel: novel, user: user})
		if !matches {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, item)
	}
	return filtered
}

func match(item any, filters value) (string, bool) {
	switch value := item.(type) {
	case pixiv.Artwork:
		if !matchesIllust(value, filters.illust) {
			return "", false
		}
		kind := string(value.Kind)
		if kind == "" {
			kind = string(pixiv.ArtworkKindIllustration)
		}
		return fmt.Sprintf("%s:%d", kind, value.ID), true
	case pixiv.Novel:
		if !matchesNovel(value, filters.novel) {
			return "", false
		}
		return fmt.Sprintf("novel:%d", value.ID), true
	case pixiv.UserPreview:
		if !matchesUser(value.User, filters.user) {
			return "", false
		}
		return fmt.Sprintf("user:%d", value.User.ID), true
	default:
		return fmt.Sprintf("%T:%v", item, item), true
	}
}

func matchesIllust(item pixiv.Artwork, filter *IllustFilter) bool {
	if filter == nil {
		return true
	}
	if filter.ID != nil && item.ID != *filter.ID {
		return false
	}
	if filter.Type != "" {
		kind := string(item.Kind)
		if kind == "" {
			kind = string(pixiv.ArtworkKindIllustration)
		}
		// 上游 kind 渲染为 "illustration"，而 MCP 枚举使用 "illust"；两者等价。
		if kind != filter.Type && (kind != string(pixiv.ArtworkKindIllustration) || filter.Type != "illust") {
			return false
		}
	}
	if filter.MinViews != nil && item.TotalViews < *filter.MinViews {
		return false
	}
	if filter.MinPages != nil && item.PageCount < *filter.MinPages {
		return false
	}
	return hasAllTags(item.Tags, filter.Tags)
}

func matchesNovel(item pixiv.Novel, filter *NovelFilter) bool {
	if filter == nil {
		return true
	}
	if filter.ID != nil && item.ID != *filter.ID {
		return false
	}
	if filter.MinViews != nil && item.TotalViews < *filter.MinViews {
		return false
	}
	return hasAllTags(item.Tags, filter.Tags)
}

func matchesUser(item pixiv.User, filter *UserFilter) bool {
	return filter == nil || filter.ID == nil || item.ID == *filter.ID
}

func hasAllTags(tags []pixiv.Tag, wanted []string) bool {
	for _, want := range wanted {
		found := false
		for _, tag := range tags {
			if tag.Name == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
