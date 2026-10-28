package sdk

import (
	"errors"
	"fmt"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// Reason classifies a failure in a form both product SDKs share and callers can
// match on. Reasons are part of the v1 contract: new reasons may be added, but
// an existing one is never removed, renamed, or reused for another meaning.
type Reason string

// InvalidArgument and the remaining reasons are the stable v1 classifications
// both product SDKs return. A reason may be added, never removed, renamed, or
// reused for another meaning.
const (
	InvalidArgument           Reason = "invalid_argument"
	InvalidCursor             Reason = "invalid_cursor"
	Unauthorized              Reason = "unauthorized"
	CredentialsExpired        Reason = "credentials_expired"
	Forbidden                 Reason = "forbidden"
	NotFound                  Reason = "not_found"
	ContentUnavailable        Reason = "content_unavailable"
	ChallengeRequired         Reason = "challenge_required"
	RateLimited               Reason = "rate_limited"
	UpstreamError             Reason = "upstream_error"
	UpstreamUnavailable       Reason = "upstream_unavailable"
	MalformedUpstreamResponse Reason = "malformed_upstream_response"
	ResourceForbidden         Reason = "resource_forbidden"
	LocalStateError           Reason = "local_state_error"
	RemovedSetting            Reason = "removed_setting"
)

// Transport identifies the layer a transport failure came from, which lets
// callers tell network problems apart from classified upstream errors.
type Transport string

// TransportHTTP and the other Transport values name the layer a failure came
// from.
const (
	TransportHTTP  Transport = "http"
	TransportTLS   Transport = "tls"
	TransportDNS   Transport = "dns"
	TransportLocal Transport = "local"
)

// RetryAdvice carries retry information derived only from verified upstream
// sources. Safe means the operation can be retried without risking a duplicate
// commit; the SDK never retries on its own. After comes from a validated
// upstream value and is meaningful only when HasAfter is true.
type RetryAdvice struct {
	Safe     bool
	After    time.Time
	HasAfter bool
}

// origin keeps diagnostic metadata outside the public method set.
type origin = logging.ErrorOrigin

// Error carries a classified, redacted failure from the product SDKs and this
// package's parsing helpers. It implements error, errors.Is, and errors.As, and
// its Unwrap chain preserves context.Canceled and context.DeadlineExceeded when
// one of those caused the failure.
//
// The error chain never contains raw URLs, request headers, response bodies,
// tokens, cookies, proxy userinfo, browser paths, or configuration content.
// The cause, when present, is always a redacted local classification.
type Error struct {
	origin
	Product    string
	Operation  string
	Reason     Reason
	Detail     string
	HTTPStatus int
	Transport  Transport
	Retry      RetryAdvice

	cause error
}

// ErrorOption adds classification fields while NewError builds an Error.
type ErrorOption func(*Error)

// WithCause attaches a redacted underlying cause. The chain must not contain
// secrets or raw upstream artifacts; product SDKs are responsible for
// classifying and redacting before wrapping.
func WithCause(cause error) ErrorOption {
	return func(e *Error) { e.cause = cause }
}

// WithHTTPStatus records the normalized upstream HTTP status; zero means
// upstream reported none.
func WithHTTPStatus(status int) ErrorOption {
	return func(e *Error) { e.HTTPStatus = status }
}

// WithTransport attaches the transport layer classification.
func WithTransport(transport Transport) ErrorOption {
	return func(e *Error) { e.Transport = transport }
}

// WithRetry attaches retry advice derived only from verified upstream sources.
func WithRetry(retry RetryAdvice) ErrorOption {
	return func(e *Error) { e.Retry = retry }
}

// WithDetail attaches a controlled, redacted detail string that further
// classifies the failure without leaking upstream artifacts.
func WithDetail(detail string) ErrorOption {
	return func(e *Error) { e.Detail = detail }
}

// NewError constructs a classified error for product and operation. product
// may be empty for errors produced by this shared package.
func NewError(product, operation string, reason Reason, opts ...ErrorOption) *Error {
	e := &Error{origin: logging.CaptureErrorOrigin(), Product: product, Operation: operation, Reason: reason}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Error renders a human-readable summary that never includes raw upstream
// artifacts; the cause is a redacted classification.
func (e *Error) Error() string {
	head := e.Product
	if head == "" {
		head = "sdk"
	}
	msg := fmt.Sprintf("%s:%s: %s", head, e.Operation, e.Reason)
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.cause != nil && e.cause.Error() != "" {
		msg += ": " + e.cause.Error()
	}
	return msg
}

// Unwrap returns the redacted cause, allowing errors.Is and errors.As to walk
// the chain (for example to match context.Canceled).
func (e *Error) Unwrap() error { return e.cause }

// Is matches target by Reason, letting a classified error match a Reason-only
// sentinel.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok || t == nil {
		return false
	}
	return t.Reason != "" && e.Reason == t.Reason
}

// ReasonOf returns the Reason of err when it is or wraps an *Error, and the
// empty string otherwise.
func ReasonOf(err error) Reason {
	var target *Error
	if errors.As(err, &target) {
		return target.Reason
	}
	return ""
}

// IsReason reports whether err is or wraps an *Error with the given reason.
func IsReason(err error, reason Reason) bool {
	return ReasonOf(err) == reason
}
