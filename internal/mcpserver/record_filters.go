package mcpserver

import (
	"context"
	"errors"
	"fmt"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// illustFilterIn、novelFilterIn 与 userFilterIn 仅出现在相应实体 list tool 的
// schema 中。筛选在每个上游批次进入逻辑分页前执行，避免把不匹配项计入 limit。
type illustFilterIn struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive illustration ID"`
	Type     string   `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
	MinPages *int     `json:"min_pages,omitempty" jsonschema:"minimum page count"`
}

type novelFilterIn struct {
	ID       *int64   `json:"id,omitempty" jsonschema:"optional positive novel ID"`
	Tags     []string `json:"tags,omitempty" jsonschema:"all exact tags required"`
	MinViews *int     `json:"min_views,omitempty" jsonschema:"minimum public view count"`
}

type userFilterIn struct {
	ID *int64 `json:"id,omitempty" jsonschema:"optional positive user ID"`
}

// 这三个 schema 固定各实体允许的筛选字段，避免错误实体的字段在 MCP 协议层被
// 悄然接受。其余由反射生成的 tool 也复用同一输入结构。
func illustFilterSchema() map[string]any {
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

func novelFilterSchema() map[string]any {
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

func userFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id": map[string]any{"type": "integer", "minimum": 1},
		},
	}
}

type recordFilterContextKey struct{}

type recordFilters struct {
	illust *illustFilterIn
	novel  *novelFilterIn
	user   *userFilterIn
}

func withIllustFilter(ctx context.Context, filter *illustFilterIn) (context.Context, error) {
	return withIllustFilterExpression(ctx, filter, "")
}

// withIllustFilterExpression 将 MCP 顶层 filter 与结构化 illust_filter 组合为
// 同一份逻辑筛选。顶层表达式不再被编译；结构化筛选仍在打开 SDK client 前校验，
// 非法输入不会产生网络请求。
func withIllustFilterExpression(ctx context.Context, filter *illustFilterIn, expression string) (context.Context, error) {
	if err := validateIllustFilter(filter); err != nil {
		return nil, err
	}
	return withRecordFilters(ctx, recordFilters{illust: filter}), nil
}

func withNovelFilter(ctx context.Context, filter *novelFilterIn) (context.Context, error) {
	if err := validateNovelFilter(filter); err != nil {
		return nil, err
	}
	return withRecordFilters(ctx, recordFilters{novel: filter}), nil
}

func withUserFilter(ctx context.Context, filter *userFilterIn) (context.Context, error) {
	if err := validateUserFilter(filter); err != nil {
		return nil, err
	}
	return withRecordFilters(ctx, recordFilters{user: filter}), nil
}

func withMixedRecordFilters(ctx context.Context, illust *illustFilterIn, novel *novelFilterIn, user *userFilterIn) (context.Context, error) {
	return withMixedRecordFiltersExpression(ctx, illust, "", novel, user)
}

// withMixedRecordFiltersExpression 使 recommended 的视觉记录使用同一条顶层
// filter；顶层表达式不再被编译，结构化筛选仍逐实体校验。
func withMixedRecordFiltersExpression(ctx context.Context, illust *illustFilterIn, expression string, novel *novelFilterIn, user *userFilterIn) (context.Context, error) {
	if err := validateIllustFilter(illust); err != nil {
		return nil, err
	}
	if err := validateNovelFilter(novel); err != nil {
		return nil, err
	}
	if err := validateUserFilter(user); err != nil {
		return nil, err
	}
	return withRecordFilters(ctx, recordFilters{illust: illust, novel: novel, user: user}), nil
}

func withRecordFilters(ctx context.Context, filters recordFilters) context.Context {
	return context.WithValue(ctx, recordFilterContextKey{}, filters)
}

func filtersFromContext(ctx context.Context) recordFilters {
	filters, _ := ctx.Value(recordFilterContextKey{}).(recordFilters)
	return filters
}

func validateIllustFilter(filter *illustFilterIn) error {
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

func validateNovelFilter(filter *novelFilterIn) error {
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

func validateUserFilter(filter *userFilterIn) error {
	if filter != nil && filter.ID != nil && *filter.ID <= 0 {
		return errors.New("user_filter.id must be positive")
	}
	return nil
}

func filterRecordPage[T any](ctx context.Context, items []T, seen map[string]struct{}) []T {
	filters := filtersFromContext(ctx)
	filtered := make([]T, 0, len(items))
	for _, item := range items {
		key, matches := matchRecordFilter(any(item), filters)
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

func matchRecordFilter(item any, filters recordFilters) (string, bool) {
	switch value := item.(type) {
	case pixiv.Artwork:
		if !matchesIllustFilter(value, filters.illust) {
			return "", false
		}
		kind := string(value.Kind)
		if kind == "" {
			kind = string(pixiv.ArtworkKindIllustration)
		}
		return fmt.Sprintf("%s:%d", kind, value.ID), true
	case pixiv.Novel:
		if !matchesNovelFilter(value, filters.novel) {
			return "", false
		}
		return fmt.Sprintf("novel:%d", value.ID), true
	case pixiv.UserPreview:
		if !matchesUserFilter(value.User, filters.user) {
			return "", false
		}
		return fmt.Sprintf("user:%d", value.User.ID), true
	default:
		return fmt.Sprintf("%T:%v", item, item), true
	}
}

func matchesIllustFilter(item pixiv.Artwork, filter *illustFilterIn) bool {
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
		if kind != filter.Type && !(kind == string(pixiv.ArtworkKindIllustration) && filter.Type == "illust") {
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

func matchesNovelFilter(item pixiv.Novel, filter *novelFilterIn) bool {
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

func matchesUserFilter(item pixiv.User, filter *userFilterIn) bool {
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
