package toolrun

import "github.com/FlanChanXwO/pixiv-cli/internal/pagination"

// ListIn 区分省略 limit（读取一个默认批次）与 limit=0（返回全部结果）。
type ListIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

// ParseListPlan 在开始 SDK 工作前校验外部输入；分页语义由 internal/pagination 拥有。
func ParseListPlan(input ListIn) (pagination.PagePlan, error) {
	return pagination.ParseInput(input.Page, input.Limit)
}
