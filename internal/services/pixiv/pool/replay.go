package pool

import (
	"errors"
	"fmt"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func retryAfterFor(err error, now time.Time) (time.Duration, bool) {
	var typed *sdk.Error
	if !errors.As(err, &typed) || typed.Reason != sdk.RateLimited || !typed.Retry.Safe || !typed.Retry.HasAfter {
		return 0, false
	}
	retryAfter := typed.Retry.After.Sub(now)
	if retryAfter <= 0 {
		return 0, false
	}
	return retryAfter, true
}

func mapSelectionError(err, lastRateLimit error, now time.Time) error {
	var selectionErr *accountpixiv.PoolSelectionError
	if !errors.As(err, &selectionErr) {
		if errors.Is(err, ErrAccountPoolExhausted) {
			return poolExhaustedError(lastRateLimit, nil, now)
		}
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError, sdk.WithDetail("account_pool_state_error"), sdk.WithCause(err))
	}
	switch selectionErr.Kind {
	case accountpixiv.PoolSelectionNoLocalAccount:
		return sdk.NewError("pixiv", "account_pool", sdk.Unauthorized, sdk.WithDetail("account_pool_no_local_account"))
	case accountpixiv.PoolSelectionNoSchedulable:
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError, sdk.WithDetail("account_pool_no_schedulable_account"))
	case accountpixiv.PoolSelectionAllFrozen:
		return poolSelectionRateLimit("account_pool_all_frozen", selectionErr.EarliestFrozenUntil, nil, now)
	case accountpixiv.PoolSelectionExhausted:
		return poolExhaustedError(lastRateLimit, selectionErr.EarliestFrozenUntil, now)
	default:
		return fmt.Errorf("account pool state store returned unknown selection kind %q", selectionErr.Kind)
	}
}

func poolSelectionRateLimit(detail string, earliest *int64, cause error, now time.Time) error {
	opts := []sdk.ErrorOption{sdk.WithDetail(detail)}
	if retry, ok := poolRetryAdvice(earliest, now); ok {
		opts = append(opts, sdk.WithRetry(retry))
	}
	if cause != nil {
		opts = append(opts, sdk.WithCause(cause))
	}
	return sdk.NewError("pixiv", "account_pool", sdk.RateLimited, opts...)
}

func poolRetryAdvice(earliest *int64, now time.Time) (sdk.RetryAdvice, bool) {
	if earliest == nil {
		return sdk.RetryAdvice{}, false
	}
	after := time.Unix(*earliest, 0)
	if !after.After(now) {
		return sdk.RetryAdvice{}, false
	}
	return sdk.RetryAdvice{Safe: true, After: after, HasAfter: true}, true
}

func poolExhaustedError(lastRateLimit error, earliest *int64, now time.Time) error {
	if lastRateLimit == nil {
		return ErrAccountPoolExhausted
	}
	classified := poolSelectionRateLimit("account_pool_exhausted", earliest, lastRateLimit, now)
	var typed *sdk.Error
	if !errors.As(classified, &typed) {
		return fmt.Errorf("%w: %w", ErrAccountPoolExhausted, classified)
	}
	return &accountPoolExhaustedError{classified: typed}
}

type accountPoolExhaustedError struct{ classified *sdk.Error }

func (e *accountPoolExhaustedError) Error() string { return e.classified.Error() }

func (e *accountPoolExhaustedError) Unwrap() []error {
	return []error{ErrAccountPoolExhausted, e.classified}
}
