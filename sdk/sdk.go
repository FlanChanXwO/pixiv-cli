// Package sdk provides classified errors and resumable pages for the product SDKs.
//
// [Error] reports failures through [Reason] and preserves underlying errors for
// errors.Is and errors.As. [Page] carries results and an opaque [Cursor].
// Product clients own the query and connection used by Page.Next.
//
// # Resuming a list
//
// After consuming a page, persist page.Cursor.String(). To resume, recreate
// the original request and use the same account. Here list is the original
// product list operation, and request contains its original query options:
//
//	request.Cursor, err = sdk.ParseCursor(saved)
//	if err != nil {
//		return err
//	}
//	page, err = list(ctx, request)
//
// Check Cursor.IsZero before resuming: an empty cursor starts a fresh query.
// During a live iteration, page.Next(ctx) keeps the client and query bound.
// JSON-decoded pages retain data and cursors; resume them through the list
// operation to restore that binding.
package sdk

import (
	"errors"
	"fmt"
	"time"
)

// Reason identifies the category of an SDK failure.
type Reason string

const (
	// InvalidArgument reports an invalid input, mismatched cursor, or unbound object.
	InvalidArgument Reason = "invalid_argument"
	// Unauthorized reports missing or rejected authentication.
	Unauthorized Reason = "unauthorized"
	// CredentialsExpired reports credentials requiring renewal or login.
	CredentialsExpired Reason = "credentials_expired"
	// Forbidden reports an upstream access restriction.
	Forbidden Reason = "forbidden"
	// NotFound reports an upstream object that does not exist.
	NotFound Reason = "not_found"
	// ContentUnavailable reports content that the upstream cannot provide.
	ContentUnavailable Reason = "content_unavailable"
	// ChallengeRequired reports an unresolved browser challenge.
	ChallengeRequired Reason = "challenge_required"
	// RateLimited reports an upstream request limit.
	RateLimited Reason = "rate_limited"
	// UpstreamError reports an upstream operation failure.
	UpstreamError Reason = "upstream_error"
	// UpstreamUnavailable reports an unavailable upstream or transport.
	UpstreamUnavailable Reason = "upstream_unavailable"
	// MalformedUpstreamResponse reports an invalid upstream response.
	MalformedUpstreamResponse Reason = "malformed_upstream_response"
	// ResourceForbidden reports a resource outside the product's allowed origins.
	ResourceForbidden Reason = "resource_forbidden"
	// LocalStateError reports a credential read, save, or other local state failure.
	LocalStateError Reason = "local_state_error"
)

// Error records an SDK failure. Operation and Message contain controlled text;
// Err preserves its cause and must be safe to expose through Unwrap. Product
// implementations classify external errors before attaching them.
// Formatting prints the summary only, including with %#v; inspect Unwrap to
// retrieve the cause. Match Reason with ReasonOf or errors.Is.
type Error struct {
	Reason     Reason
	Operation  string
	Message    string
	HTTPStatus int            // Zero when no HTTP response was received.
	RetryAfter *time.Duration // Present only when the upstream supplied a valid Retry-After.
	Err        error          `json:"-"` // Inspect with Unwrap; causes are excluded from JSON diagnostics.
}

func newError(reason Reason, operation, message string, cause error) *Error {
	return &Error{Reason: reason, Operation: operation, Message: message, Err: cause}
}

// Error returns the classification and controlled summary, without the cause.
func (e *Error) Error() string {
	text := string(e.Reason)
	if e.Operation != "" {
		text = e.Operation + ": " + text
	}
	if e.Message != "" {
		text += ": " + e.Message
	}
	return text
}

// Format keeps the cause out of formatted diagnostics, including %#v.
func (e *Error) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		fmt.Fprintf(state, "%q", e.Error())
		return
	}
	fmt.Fprint(state, e.Error())
}

// Unwrap returns the underlying error, including context cancellation or a
// persistence failure when applicable.
func (e *Error) Unwrap() error { return e.Err }

// Is matches a nonempty Reason on another Error.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && other.Reason != "" && e.Reason == other.Reason
}

// ReasonOf returns the first SDK classification in err's chain, or "".
func ReasonOf(err error) Reason {
	var failure *Error
	if errors.As(err, &failure) {
		return failure.Reason
	}
	return ""
}
