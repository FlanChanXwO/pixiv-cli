// Parse MCP page/limit inputs and construct logical-page output metadata.

package toolrun

import (
	"errors"
	"math"
)

// ListIn distinguishes an omitted limit (one batch) from zero (all results).
type ListIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

// ListPlan is a validated logical-page request.
type ListPlan struct {
	Page     int
	Limit    int
	OneBatch bool
	Skip     int
}

// PaginationOut contains the shared logical-page wire fields.
type PaginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

// ParseListPlan validates external list input before SDK work begins.
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

// ListPagination describes the returned logical page; batch cursors stay product-owned.
func ListPagination(plan ListPlan, limit *int, returned int, hasMore bool) PaginationOut {
	out := PaginationOut{Page: plan.Page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.Page < math.MaxInt {
		next := plan.Page + 1
		out.NextPage = &next
	}
	return out
}
