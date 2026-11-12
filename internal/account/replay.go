package account

import (
	"errors"
	"fmt"
	"time"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func retryAfterFor(err error, now time.Time) (time.Duration, bool) {
	var current *sdk.Error
	if errors.As(err, &current) && current.Reason == sdk.RateLimited && current.RetryAfter != nil && *current.RetryAfter > 0 {
		return *current.RetryAfter, true
	}
	var typed *legacysdk.Error
	if !errors.As(err, &typed) || typed.Reason != legacysdk.RateLimited || !typed.Retry.Safe || !typed.Retry.HasAfter {
		return 0, false
	}
	retryAfter := typed.Retry.After.Sub(now)
	if retryAfter <= 0 {
		return 0, false
	}
	return retryAfter, true
}

func mapSelectionError(err, lastRateLimit error, now time.Time) error {
	var selectionErr *PoolSelectionError
	if !errors.As(err, &selectionErr) {
		if errors.Is(err, ErrAccountPoolExhausted) {
			return poolExhaustedError(lastRateLimit, nil, now)
		}
		return legacysdk.NewError("pixiv", "account_pool", legacysdk.LocalStateError, legacysdk.WithDetail("account_pool_state_error"), legacysdk.WithCause(err))
	}
	switch selectionErr.Reason {
	case PoolSelectionNoLocalAccount:
		return legacysdk.NewError("pixiv", "account_pool", legacysdk.Unauthorized, legacysdk.WithDetail("account_pool_no_local_account"))
	case PoolSelectionNoSchedulable:
		return legacysdk.NewError("pixiv", "account_pool", legacysdk.LocalStateError, legacysdk.WithDetail("account_pool_no_schedulable_account"))
	case PoolSelectionAllFrozen:
		return poolSelectionRateLimit("account_pool_all_frozen", selectionErr.EarliestFrozenUntil, nil, now)
	case PoolSelectionExhausted:
		return poolExhaustedError(lastRateLimit, selectionErr.EarliestFrozenUntil, now)
	default:
		return fmt.Errorf("account pool state store returned unknown selection reason %q", selectionErr.Reason)
	}
}

func poolSelectionRateLimit(detail string, earliest *int64, cause error, now time.Time) *legacysdk.Error {
	opts := []legacysdk.ErrorOption{legacysdk.WithDetail(detail)}
	if retry, ok := poolRetryAdvice(earliest, now); ok {
		opts = append(opts, legacysdk.WithRetry(retry))
	}
	if cause != nil {
		opts = append(opts, legacysdk.WithCause(cause))
	}
	return legacysdk.NewError("pixiv", "account_pool", legacysdk.RateLimited, opts...)
}

func poolRetryAdvice(earliest *int64, now time.Time) (legacysdk.RetryAdvice, bool) {
	if earliest == nil {
		return legacysdk.RetryAdvice{}, false
	}
	after := time.Unix(*earliest, 0)
	if !after.After(now) {
		return legacysdk.RetryAdvice{}, false
	}
	return legacysdk.RetryAdvice{Safe: true, After: after, HasAfter: true}, true
}

func poolExhaustedError(lastRateLimit error, earliest *int64, now time.Time) error {
	if lastRateLimit == nil {
		return ErrAccountPoolExhausted
	}
	classified := poolSelectionRateLimit("account_pool_exhausted", earliest, lastRateLimit, now)
	return &accountPoolExhaustedError{classified: classified}
}

type accountPoolExhaustedError struct{ classified *legacysdk.Error }

func (e *accountPoolExhaustedError) Error() string { return e.classified.Error() }

func (e *accountPoolExhaustedError) Unwrap() []error {
	return []error{ErrAccountPoolExhausted, e.classified}
}
