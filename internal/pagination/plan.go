package pagination

import (
	"context"
	"errors"
	"math"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// 校验失败的原因；调用方按自己的输出契约翻译其中需要改写的文本。
var (
	// ErrLimitNegative 表示 limit 为负数。
	ErrLimitNegative = errors.New("limit must be zero or a positive integer")
	// ErrPageNotPositive 表示显式给出的 page 不是正整数。
	ErrPageNotPositive = errors.New("page must be a positive integer")
	// ErrPageRequiresLimit 表示给了 page 却没有正数 limit。
	ErrPageRequiresLimit = errors.New("--page requires --limit to be a positive integer")
	// ErrOffsetOverflow 表示 page 与 limit 相乘溢出。
	ErrOffsetOverflow = errors.New("page and limit overflow the logical result offset")
)

// ParsePlan 校验 limit/page 的组合并生成逻辑分页计划。limitSet 与 pageSet 区分“省略”
// 与显式零值：省略 limit 时只读一批，显式 limit=0 读取全部。
func ParsePlan(limit, page int, limitSet, pageSet bool) (PagePlan, error) {
	if limit < 0 {
		return PagePlan{}, ErrLimitNegative
	}
	if pageSet && page <= 0 {
		return PagePlan{}, ErrPageNotPositive
	}
	if page > 0 {
		if limit <= 0 {
			return PagePlan{}, ErrPageRequiresLimit
		}
		if page-1 > math.MaxInt/limit {
			return PagePlan{}, ErrOffsetOverflow
		}
		return PagePlan{Page: page, Limit: limit, Skip: (page - 1) * limit}, nil
	}
	return PagePlan{Page: 1, Limit: limit, OneBatch: !limitSet}, nil
}

// PushDownSkip 把逻辑跳过量转成上游 offset，并清空本地 skip。只有上游原始结果与
// 逻辑结果一一对应时才能这样调用。
func (p PagePlan) PushDownSkip() (PagePlan, int) {
	offset := p.Skip
	p.Skip = 0
	return p, offset
}

// PaginationOut 保存两个产品共用的分页输出字段；NextPage 只在已知下一页时非 nil。
type PaginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

// ListPagination 描述本次返回的逻辑页；批次 cursor 由各产品自己维护。
func ListPagination(plan PagePlan, limit *int, returned int, hasMore bool) PaginationOut {
	out := PaginationOut{Page: plan.Page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.Page < math.MaxInt {
		next := plan.Page + 1
		out.NextPage = &next
	}
	return out
}

// PageItems 把分页计划交给共享遍历，供自带内联读取的命令使用。
func PageItems[T any](ctx context.Context, plan PagePlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := TraversePages(ctx, plan, fetch, consume)
	return err
}

// ParseInput 校验 MCP 工具的 page/limit 输入。省略 limit 时读一批，limit=0 读取全部；
// 文本沿用 MCP 已发布的错误措辞。
func ParseInput(page, limit *int) (PagePlan, error) {
	if page != nil && *page <= 0 {
		return PagePlan{}, ErrPageNotPositive
	}
	if limit != nil && *limit < 0 {
		return PagePlan{}, ErrLimitNegative
	}
	if page != nil {
		if limit == nil || *limit <= 0 {
			return PagePlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *page-1 > math.MaxInt / *limit {
			return PagePlan{}, ErrOffsetOverflow
		}
		return PagePlan{Page: *page, Limit: *limit, Skip: (*page - 1) * *limit}, nil
	}
	if limit == nil {
		return PagePlan{Page: 1, OneBatch: true}, nil
	}
	return PagePlan{Page: 1, Limit: *limit}, nil
}
