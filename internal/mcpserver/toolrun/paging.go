package toolrun

import (
	"errors"
	"math"
)

// ListIn 区分省略 limit（读取一个默认批次）与 limit=0（返回全部结果）。
type ListIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

// ListPlan 保存校验后的逻辑分页请求；OneBatch 为 true 时不按 limit 截断。
type ListPlan struct {
	Page     int
	Limit    int
	OneBatch bool
	Skip     int
}

// PaginationOut 保存两个产品共用的分页输出字段；NextPage 只在已知下一页时非 nil。
type PaginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

// ParseListPlan 在开始 SDK 工作前校验外部输入；page 必须配正数 limit，溢出时返回错误。
func ParseListPlan(input ListIn) (ListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return ListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return ListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return ListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return ListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return ListPlan{Page: *input.Page, Limit: *input.Limit, Skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return ListPlan{Page: 1, OneBatch: true}, nil
	}
	return ListPlan{Page: 1, Limit: *input.Limit}, nil
}

// ListPagination 描述本次返回的逻辑页；批次 cursor 由各产品自己维护。
func ListPagination(plan ListPlan, limit *int, returned int, hasMore bool) PaginationOut {
	out := PaginationOut{Page: plan.Page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.Page < math.MaxInt {
		next := plan.Page + 1
		out.NextPage = &next
	}
	return out
}
