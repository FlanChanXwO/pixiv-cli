// Bookmark range semantics shared by the CLI and MCP artwork search adapters.
package searchfilter

import "errors"

// BookmarkStrategy is the resolved bookmark-count filter strategy.
type BookmarkStrategy string

// BookmarkStrategy values accepted on the command line and in tool input.
const (
	BookmarkStrategyAuto       BookmarkStrategy = "auto"
	BookmarkStrategyServer     BookmarkStrategy = "server"
	BookmarkStrategyLocal      BookmarkStrategy = "local"
	BookmarkStrategyBestEffort BookmarkStrategy = "best_effort"
)

// BookmarkMembership is the account's verified membership evidence. Only the
// CLI can distinguish premium from non-premium; MCP always reports unknown.
type BookmarkMembership string

// BookmarkMembership values reported in filter metadata.
const (
	BookmarkMembershipPremium    BookmarkMembership = "premium"
	BookmarkMembershipNonPremium BookmarkMembership = "non_premium"
	BookmarkMembershipUnknown    BookmarkMembership = "unknown"
)

// BookmarkCompleteness reports whether the returned window covers the source
// stream for the applied strategy.
type BookmarkCompleteness string

// BookmarkCompleteness values reported in filter metadata.
const (
	BookmarkCompletenessPartial           BookmarkCompleteness = "partial"
	BookmarkCompletenessCompleteForSource BookmarkCompleteness = "complete_for_source"
)

// Bookmark range rule failures. Callers map these to their own public error
// class and keep their existing user-facing text.
var (
	ErrBookmarkMinimum    = errors.New("bookmark minimum must be zero or positive")
	ErrBookmarkMaximum    = errors.New("bookmark maximum must be zero or positive")
	ErrBookmarkRangeOrder = errors.New("bookmark minimum must not exceed maximum")
	ErrBookmarkMembership = errors.New("unknown bookmark membership")
	ErrBookmarkStrategy   = errors.New("unknown bookmark filter strategy")
	ErrBookmarkServer     = errors.New("server bookmark strategy requires verified premium membership and evidence")
	ErrBookmarkNonPremium = errors.New("server bookmark strategy is not available for non-premium membership")
)

// ValidateBookmarkRange rejects negative bounds and an inverted range.
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

// NormalizeBookmarkMembership turns absent evidence into unknown. Premium
// claims require verification, so only the two verified values pass through.
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

// ResolveBookmarkStrategy maps the requested strategy to the one the caller
// can actually execute. auto only narrows to local when a range exists.
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

// BookmarkRange is one resolved bookmark-count filter.
type BookmarkRange struct {
	Min        *int
	Max        *int
	Membership BookmarkMembership
	Strategy   BookmarkStrategy
}

// Active reports whether either bound narrows the candidate stream.
func (r BookmarkRange) Active() bool { return r.Min != nil || r.Max != nil }

// Local reports whether the upstream bounds must be cleared so the filter can
// enumerate the normal candidate stream and verify counts itself.
func (r BookmarkRange) Local() bool { return r.Strategy == BookmarkStrategyLocal }

// CursorContext returns the continuation binding for this filter.
func (r BookmarkRange) CursorContext() string {
	return BookmarkContext(r.Min, r.Max, string(r.Strategy))
}

// Matches reports whether a verified bookmark count falls inside the range.
func (r BookmarkRange) Matches(totalBookmarks int) bool {
	return (r.Min == nil || totalBookmarks >= *r.Min) && (r.Max == nil || totalBookmarks <= *r.Max)
}

// Completeness reports whether the window covers the source stream. more is
// true when the traversal stopped before exhausting the source.
func (r BookmarkRange) Completeness(more bool) BookmarkCompleteness {
	if r.Strategy == BookmarkStrategyBestEffort || more {
		return BookmarkCompletenessPartial
	}
	return BookmarkCompletenessCompleteForSource
}

// Outcome returns filter metadata with bounds copied for the caller.
func (r BookmarkRange) Outcome(more bool) BookmarkOutcome {
	return BookmarkOutcome{
		Min:          cloneInt(r.Min),
		Max:          cloneInt(r.Max),
		Membership:   r.Membership,
		Strategy:     r.Strategy,
		Completeness: r.Completeness(more),
	}
}

// BookmarkOutcome is the filter metadata published next to search results.
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
