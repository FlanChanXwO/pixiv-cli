package account

import (
	"errors"
	"fmt"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func retryAfterFor(err error) (time.Duration, bool) {
	var current *sdk.Error
	if errors.As(err, &current) && current.Reason == sdk.RateLimited && current.RetryAfter != nil && *current.RetryAfter > 0 {
		return *current.RetryAfter, true
	}
	return 0, false
}

func mapSelectionError(err, lastRateLimit error, now time.Time) error {
	var selectionErr *PoolSelectionError
	if !errors.As(err, &selectionErr) {
		if errors.Is(err, ErrAccountPoolExhausted) {
			return poolExhaustedError(lastRateLimit, nil, now)
		}
		failure := &sdk.Error{Operation: "pixiv:account_pool", Reason: sdk.LocalStateError, Message: "account_pool_state_error"}
		return fmt.Errorf("%w: %w", failure, err)
	}
	switch selectionErr.Reason {
	case PoolSelectionNoLocalAccount:
		return &sdk.Error{Operation: "pixiv:account_pool", Reason: sdk.Unauthorized, Message: "account_pool_no_local_account"}
	case PoolSelectionNoSchedulable:
		return &sdk.Error{Operation: "pixiv:account_pool", Reason: sdk.LocalStateError, Message: "account_pool_no_schedulable_account"}
	case PoolSelectionAllFrozen:
		return poolSelectionRateLimit("account_pool_all_frozen", selectionErr.EarliestFrozenUntil, nil, now)
	case PoolSelectionExhausted:
		return poolExhaustedError(lastRateLimit, selectionErr.EarliestFrozenUntil, now)
	default:
		return i18n.Errorf("error.account_pool_state_store_returned_unknown_selection_reason", selectionErr.Reason)
	}
}

func poolSelectionRateLimit(detail string, earliest *int64, cause error, now time.Time) *sdk.Error {
	failure := &sdk.Error{Operation: "pixiv:account_pool", Reason: sdk.RateLimited, Message: detail, Err: cause}
	if earliest != nil {
		if wait := time.Unix(*earliest, 0).Sub(now); wait > 0 {
			failure.RetryAfter = &wait
		}
	}
	return failure
}

func poolExhaustedError(lastRateLimit error, earliest *int64, now time.Time) error {
	if lastRateLimit == nil {
		return ErrAccountPoolExhausted
	}
	classified := poolSelectionRateLimit("account_pool_exhausted", earliest, lastRateLimit, now)
	return &accountPoolExhaustedError{classified: classified}
}

type accountPoolExhaustedError struct{ classified *sdk.Error }

func (e *accountPoolExhaustedError) Error() string {
	return fmt.Sprintf("%s: %s", e.classified.Error(), e.classified.Err.Error())
}

func (e *accountPoolExhaustedError) Unwrap() []error {
	return []error{ErrAccountPoolExhausted, e.classified}
}
