// Package searchfilter 实现搜索结果使用的本地内容筛选与收藏数筛选策略。
//
// 筛选只在本地匹配，不把 rating 或收藏范围映射成 upstream 请求参数；每种筛选条件
// 都可以生成稳定的 continuation binding。
//
// 包的组成：[Filter] 按 rating 与 content type 匹配作品，[BookmarkRange] 及其
// [BookmarkStrategy]、[BookmarkMembership] 决定收藏数范围如何执行，
// [BookmarkContext] 与 [Filter.CursorContext] 生成 continuation 摘要。
package searchfilter

import "errors"

// BookmarkStrategy 枚举已解析的收藏数筛选策略。
//
// 取值来自命令行与 tool 输入；[ResolveBookmarkStrategy] 负责把不可执行的请求收窄。
type BookmarkStrategy string

// BookmarkStrategyAuto 让 [ResolveBookmarkStrategy] 决定；server 走 upstream 计数，
// local 在本地核对每个候选，best_effort 允许结果窗口不完整。
const (
	BookmarkStrategyAuto       BookmarkStrategy = "auto"
	BookmarkStrategyServer     BookmarkStrategy = "server"
	BookmarkStrategyLocal      BookmarkStrategy = "local"
	BookmarkStrategyBestEffort BookmarkStrategy = "best_effort"
)

// BookmarkMembership 记录账号会员身份的可信证据。只有 CLI 能区分 premium 与
// non-premium，MCP 始终报告 unknown。
type BookmarkMembership string

// BookmarkMembershipPremium 与 BookmarkMembershipNonPremium 必须来自验证过的证据；
// BookmarkMembershipUnknown 表示没有证据。
const (
	BookmarkMembershipPremium    BookmarkMembership = "premium"
	BookmarkMembershipNonPremium BookmarkMembership = "non_premium"
	BookmarkMembershipUnknown    BookmarkMembership = "unknown"
)

// BookmarkCompleteness 描述返回窗口在当前策略下是否覆盖了来源流。
type BookmarkCompleteness string

// BookmarkCompletenessPartial 表示窗口可能缺少后续结果；BookmarkCompletenessCompleteForSource
// 表示已覆盖来源流。
const (
	BookmarkCompletenessPartial           BookmarkCompleteness = "partial"
	BookmarkCompletenessCompleteForSource BookmarkCompleteness = "complete_for_source"
)

// 收藏范围规则错误；调用方映射到自己的公开错误类别，并保留现有用户可见文案。
var (
	ErrBookmarkMinimum    = errors.New("bookmark minimum must be zero or positive")
	ErrBookmarkMaximum    = errors.New("bookmark maximum must be zero or positive")
	ErrBookmarkRangeOrder = errors.New("bookmark minimum must not exceed maximum")
	ErrBookmarkMembership = errors.New("unknown bookmark membership")
	ErrBookmarkStrategy   = errors.New("unknown bookmark filter strategy")
	ErrBookmarkServer     = errors.New("server bookmark strategy requires verified premium membership and evidence")
	ErrBookmarkNonPremium = errors.New("server bookmark strategy is not available for non-premium membership")
)

// ValidateBookmarkRange 拒绝负数边界与 min > max；nil 表示该侧不设限。
func ValidateBookmarkRange(min, max *int) error {
	if min != nil && *min < 0 {
		return ErrBookmarkMinimum
	}
	if max != nil && *max < 0 {
		return ErrBookmarkMaximum
	}
	if min != nil && max != nil && *min > *max {
		return ErrBookmarkRangeOrder
	}
	return nil
}

// NormalizeBookmarkMembership 把缺失的证据归一为 unknown。premium 声称必须先验证，
// 因此只有两个已验证取值能通过。
func NormalizeBookmarkMembership(value string) (BookmarkMembership, error) {
	switch BookmarkMembership(value) {
	case "", BookmarkMembershipUnknown:
		return BookmarkMembershipUnknown, nil
	case BookmarkMembershipPremium, BookmarkMembershipNonPremium:
		return BookmarkMembership(value), nil
	default:
		return "", ErrBookmarkMembership
	}
}

// ResolveBookmarkStrategy 把请求的策略解析为调用方实际能执行的策略：auto 只在存在范围
// 时收窄为 local。
func ResolveBookmarkStrategy(requested string, membership BookmarkMembership, hasRange bool) (BookmarkStrategy, error) {
	strategy := BookmarkStrategy(requested)
	if strategy == "" {
		strategy = BookmarkStrategyAuto
	}
	switch strategy {
	case BookmarkStrategyAuto:
		if hasRange {
			return BookmarkStrategyLocal, nil
		}
		return BookmarkStrategyAuto, nil
	case BookmarkStrategyLocal, BookmarkStrategyBestEffort:
		return strategy, nil
	case BookmarkStrategyServer:
		if membership == BookmarkMembershipNonPremium {
			return "", ErrBookmarkNonPremium
		}
		return "", ErrBookmarkServer
	default:
		return "", ErrBookmarkStrategy
	}
}

// BookmarkRange 保存一次已解析的收藏数筛选条件。
type BookmarkRange struct {
	Min        *int
	Max        *int
	Membership BookmarkMembership
	Strategy   BookmarkStrategy
}

// Active 报告是否至少设置了一个边界。
func (r BookmarkRange) Active() bool { return r.Min != nil || r.Max != nil }

// Local 报告是否必须清空 upstream 边界，让筛选器枚举正常候选流并自行核对计数。
func (r BookmarkRange) Local() bool { return r.Strategy == BookmarkStrategyLocal }

// CursorContext 返回该条件的 continuation 摘要，不暴露原始边界。
func (r BookmarkRange) CursorContext() string {
	return BookmarkContext(r.Min, r.Max, string(r.Strategy))
}

// Matches 报告已核实的收藏数是否落在此范围内。
func (r BookmarkRange) Matches(totalBookmarks int) bool {
	return (r.Min == nil || totalBookmarks >= *r.Min) && (r.Max == nil || totalBookmarks <= *r.Max)
}

// Completeness 报告窗口是否覆盖来源流；more 表示遍历在耗尽来源前停止。
func (r BookmarkRange) Completeness(more bool) BookmarkCompleteness {
	if r.Strategy == BookmarkStrategyBestEffort || more {
		return BookmarkCompletenessPartial
	}
	return BookmarkCompletenessCompleteForSource
}

// Outcome 返回带副本边界的筛选元数据；调用方修改结果不影响 BookmarkRange。
func (r BookmarkRange) Outcome(more bool) BookmarkOutcome {
	return BookmarkOutcome{
		Min:          cloneInt(r.Min),
		Max:          cloneInt(r.Max),
		Membership:   r.Membership,
		Strategy:     r.Strategy,
		Completeness: r.Completeness(more),
	}
}

// BookmarkOutcome 承载与搜索结果一起发布的筛选元数据。
type BookmarkOutcome struct {
	Min          *int
	Max          *int
	Membership   BookmarkMembership
	Strategy     BookmarkStrategy
	Completeness BookmarkCompleteness
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
