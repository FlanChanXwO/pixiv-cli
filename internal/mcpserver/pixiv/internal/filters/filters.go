// Package filters 实现 Pixiv MCP 列表工具的本地筛选：读取调用方通过 context 传入的
// 结构化条件，在每个上游批次进入逻辑分页前过滤并按实体身份去重。
//
// 包的组成：[IllustFilter]、[NovelFilter] 与 [UserFilter] 是各实体公开的筛选字段，
// [WithIllustFilter] 等函数校验后写入 context，[FilterPage] 与 [MatchesIllust] 执行
// 匹配。筛选发生在 limit 计数之前，未命中的条目不会占位。
package filters

import (
	"context"
	"errors"
	"fmt"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// IllustFilter 描述 artwork 列表的本地筛选条件；字段为零值时该条件不生效。
type IllustFilter struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive illustration ID"`
	Type     string   `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
	MinPages *int     `json:"min_pages,omitempty" jsonschema:"minimum page count"`
}

// NovelFilter 描述 novel 列表的本地筛选条件；字段为零值时该条件不生效。
type NovelFilter struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive novel ID"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
}

// UserFilter 描述 user 列表的本地筛选条件；ID 为空时匹配所有用户。
type UserFilter struct {
	ID *int64 `json:"id,omitempty" jsonschema:"optional positive user ID"`
}

// IllustFilterSchema 返回 artwork 筛选的封闭 schema，其他实体的字段在这里会被拒绝。
func IllustFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":        map[string]any{"type": "integer", "minimum": 1},
			"type":      map[string]any{"type": "string", "enum": []string{string(pixiv.SearchContentTypeIllust), string(pixiv.SearchContentTypeManga), string(pixiv.SearchContentTypeUgoira)}},
			"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"min_views": map[string]any{"type": "integer", "minimum": 0},
			"min_pages": map[string]any{"type": "integer", "minimum": 0},
		},
	}
}

// NovelFilterSchema 返回 novel 筛选的封闭 schema。
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

// UserFilterSchema 返回 user 筛选的封闭 schema。
func UserFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id": map[string]any{"type": "integer", "minimum": 1},
		},
	}
}

type contextKey struct{}

type value struct {
	illust *IllustFilter
	novel  *NovelFilter
	user   *UserFilter
}

// WithIllustFilter 校验 artwork 筛选并写入返回的 context；非法输入在打开 SDK client
// 之前就返回错误。
func WithIllustFilter(ctx context.Context, filter *IllustFilter) (context.Context, error) {
	if err := validateIllustFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{illust: filter}), nil
}

// WithNovelFilter 校验 novel 筛选并写入返回的 context。
func WithNovelFilter(ctx context.Context, filter *NovelFilter) (context.Context, error) {
	if err := validateNovelFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{novel: filter}), nil
}

// WithUserFilter 校验 user 筛选并写入返回的 context。
func WithUserFilter(ctx context.Context, filter *UserFilter) (context.Context, error) {
	if err := validateUserFilter(filter); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{user: filter}), nil
}

// WithMixedFilters 校验 recommended 三类实体的筛选并一次性写入 context。
func WithMixedFilters(ctx context.Context, illust *IllustFilter, novel *NovelFilter, user *UserFilter) (context.Context, error) {
	if err := validateIllustFilter(illust); err != nil {
		return nil, err
	}
	if err := validateNovelFilter(novel); err != nil {
		return nil, err
	}
	if err := validateUserFilter(user); err != nil {
		return nil, err
	}
	return withFilters(ctx, value{illust: illust, novel: novel, user: user}), nil
}

func withFilters(ctx context.Context, filters value) context.Context {
	return context.WithValue(ctx, contextKey{}, filters)
}

// fromContext 读取 context 中的筛选；没有设置时三个值都为 nil。
func fromContext(ctx context.Context) (illust *IllustFilter, novel *NovelFilter, user *UserFilter) {
	filters, _ := ctx.Value(contextKey{}).(value)
	return filters.illust, filters.novel, filters.user
}

// validateIllustFilter 允许 nil，其余字段按公开 schema 的取值范围校验。
func validateIllustFilter(filter *IllustFilter) error {
	if filter == nil {
		return nil
	}
	if filter.ID != nil && *filter.ID <= 0 {
		return errors.New("illust_filter.id must be positive")
	}
	if filter.Type != "" && filter.Type != string(pixiv.SearchContentTypeIllust) && filter.Type != string(pixiv.SearchContentTypeManga) && filter.Type != string(pixiv.SearchContentTypeUgoira) {
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

// validateNovelFilter 允许 nil，其余字段按公开 schema 的取值范围校验。
func validateNovelFilter(filter *NovelFilter) error {
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

// validateUserFilter 允许 nil，其余字段按公开 schema 的取值范围校验。
func validateUserFilter(filter *UserFilter) error {
	if filter != nil && filter.ID != nil && *filter.ID <= 0 {
		return errors.New("user_filter.id must be positive")
	}
	return nil
}

// FilterPage 筛选单个上游批次，并把新出现的实体身份写入 seen。返回的切片不包含重复，
// 也可能为空；空结果不计入 limit。
func FilterPage[T any](ctx context.Context, items []T, seen map[string]struct{}) []T {
	illust, novel, user := fromContext(ctx)
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
		if !MatchesIllust(value, filters.illust) {
			return "", false
		}
		kind := string(value.Kind)
		if kind == "" || kind == string(pixiv.ArtworkKindIllustration) {
			kind = string(pixiv.SearchContentTypeIllust)
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

// MatchesIllust 按条件匹配一条作品；filter 为 nil 时匹配所有作品。
func MatchesIllust(item pixiv.Artwork, filter *IllustFilter) bool {
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
		if kind == string(pixiv.ArtworkKindIllustration) {
			kind = string(pixiv.SearchContentTypeIllust)
		}
		if kind != filter.Type {
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
