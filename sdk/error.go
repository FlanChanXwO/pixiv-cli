package sdk

import (
	"errors"
	"fmt"
	"time"
)

// Code is a stable, machine-readable error classification shared by both
// product SDKs. Codes are part of the v1 compatibility contract: a code may
// only be added, never removed, renamed, or reused for a different meaning.
type Code string

// The v1 error codes are the stable, machine-readable classifications shared by
// both product SDKs.
const (
	CodeInvalidArgument           Code = "invalid_argument"
	CodeInvalidCursor             Code = "invalid_cursor"
	CodeUnauthorized              Code = "unauthorized"
	CodeCredentialsExpired        Code = "credentials_expired"
	CodeForbidden                 Code = "forbidden"
	CodeNotFound                  Code = "not_found"
	CodeContentUnavailable        Code = "content_unavailable"
	CodeChallengeRequired         Code = "challenge_required"
	CodeRateLimited               Code = "rate_limited"
	CodeUpstreamError             Code = "upstream_error"
	CodeUpstreamUnavailable       Code = "upstream_unavailable"
	CodeMalformedUpstreamResponse Code = "malformed_upstream_response"
	CodeResourceForbidden         Code = "resource_forbidden"
	CodeLocalStateError           Code = "local_state_error"
	CodeRemovedSetting            Code = "removed_setting"
)

// Transport classifies the transport layer a failure originated in. It helps
// callers distinguish network-level failures from classified upstream errors.
type Transport string

// Transport kinds classify the transport layer a failure originated in.
const (
	TransportHTTP  Transport = "http"
	TransportTLS   Transport = "tls"
	TransportDNS   Transport = "dns"
	TransportLocal Transport = "local"
)

// RetryAdvice carries retry information derived only from verified upstream
// sources. Safe expresses that the operation can be safely retried from an
// operation-commit perspective; it does not mean the SDK retries automatically.
// After is only populated from a validated upstream value and is meaningful
// when HasAfter is true.
type RetryAdvice struct {
	Safe     bool
	After    time.Time
	HasAfter bool
}

// Error is a classified, redacted error returned by the product SDKs and by
// this package's parsing helpers. It implements error, errors.Is, and
// errors.As, and its Unwrap chain preserves context.Canceled and
// context.DeadlineExceeded when one of those caused the failure.
//
// The error chain never contains raw URLs, request headers, response bodies,
// tokens, cookies, proxy userinfo, browser paths, or configuration content.
// The cause, when present, is always a redacted local classification.
type Error struct {
	Product    string
	Operation  string
	Code       Code
	Detail     string
	HTTPStatus int
	Transport  Transport
	Retry      RetryAdvice

	cause error
}

// ErrorOption configures a classified error during construction.
type ErrorOption func(*Error)

// WithCause attaches a redacted underlying cause. The chain must not contain
// secrets or raw upstream artifacts; product SDKs are responsible for
// classifying and redacting before wrapping.
func WithCause(cause error) ErrorOption {
	return func(e *Error) { e.cause = cause }
}

// WithHTTPStatus attaches the normalized upstream HTTP status code. Zero is
// treated as "not provided".
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
func NewError(product, operation string, code Code, opts ...ErrorOption) *Error {
	e := &Error{Product: product, Operation: operation, Code: code}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Error returns a safe, human-readable summary. It never includes raw upstream
// artifacts; the cause is expected to be a redacted classification.
func (e *Error) Error() string {
	head := e.Product
	if head == "" {
		head = "sdk"
	}
	msg := fmt.Sprintf("%s:%s: %s", head, e.Operation, e.Code)
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

// Is reports whether target is an *Error with the same Code. This lets a
// classified error match a Code-only sentinel.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return t.Code != "" && e.Code == t.Code
}

// CodeOf returns the sdk.Code of err when it is or wraps an *Error, and the
// empty string otherwise.
func CodeOf(err error) Code {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

// IsCode reports whether err is or wraps an *Error with the given code.
func IsCode(err error, code Code) bool {
	return CodeOf(err) == code
}
