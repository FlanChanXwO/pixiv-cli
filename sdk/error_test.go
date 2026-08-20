package sdk_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestErrorFormatIncludesProductOperationAndReason(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.NotFound)
	msg := err.Error()
	if !strings.Contains(msg, "pixiv") || !strings.Contains(msg, "Artwork") || !strings.Contains(msg, "not_found") {
		t.Fatalf("Error() = %q", msg)
	}
}

func TestErrorDetailAndCause(t *testing.T) {
	cause := errors.New("redacted local classification")
	err := sdk.NewError("pixiv", "Artwork", sdk.UpstreamError,
		sdk.WithDetail("status 502"),
		sdk.WithCause(cause),
		sdk.WithHTTPStatus(502),
		sdk.WithTransport(sdk.TransportHTTP),
	)
	msg := err.Error()
	if !strings.Contains(msg, "status 502") || !strings.Contains(msg, "redacted local classification") {
		t.Fatalf("Error() = %q", msg)
	}
	if err.HTTPStatus != 502 {
		t.Fatalf("HTTPStatus = %d", err.HTTPStatus)
	}
	if err.Transport != sdk.TransportHTTP {
		t.Fatalf("Transport = %q", err.Transport)
	}
}

func TestErrorIsReason(t *testing.T) {
	err := sdk.NewError("pixiv", "SearchArtworks", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true}))
	if !err.Retry.Safe {
		t.Fatal("retry advice lost")
	}
	if sdk.ReasonOf(err) != sdk.RateLimited {
		t.Fatalf("ReasonOf = %q", sdk.ReasonOf(err))
	}
	if !sdk.IsReason(err, sdk.RateLimited) {
		t.Fatal("IsReason should match")
	}
	if sdk.IsReason(err, sdk.NotFound) {
		t.Fatal("IsReason should not match another reason")
	}
}

func TestErrorErrorsIsMatchesReasonSentinel(t *testing.T) {
	err := sdk.NewError("pixiv", "User", sdk.Forbidden)
	sentinel := &sdk.Error{Reason: sdk.Forbidden}
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is should match reason sentinel")
	}
	if errors.Is(err, &sdk.Error{Reason: sdk.NotFound}) {
		t.Fatal("errors.Is should not match a different reason")
	}
}

func TestErrorPreservesContextChain(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.UpstreamError,
		sdk.WithCause(context.Canceled),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context.Canceled not preserved in chain")
	}

	deadline := sdk.NewError("pixiv", "Artwork", sdk.UpstreamError,
		sdk.WithCause(context.DeadlineExceeded),
	)
	if !errors.Is(deadline, context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded not preserved in chain")
	}
}

func TestErrorErrorsAs(t *testing.T) {
	wrapped := sdk.NewError("fanbox", "Post", sdk.ContentUnavailable)
	outer := sdk.NewError("fanbox", "Post", sdk.ContentUnavailable, sdk.WithCause(wrapped))
	var target *sdk.Error
	if !errors.As(outer, &target) {
		t.Fatal("errors.As should find *sdk.Error")
	}
	if target.Reason != sdk.ContentUnavailable {
		t.Fatalf("As reason = %q", target.Reason)
	}
}

func TestErrorRetryAfter(t *testing.T) {
	after := time.Now().Add(30 * time.Second)
	err := sdk.NewError("pixiv", "SearchArtworks", sdk.RateLimited,
		sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: after, HasAfter: true}),
	)
	if !err.Retry.HasAfter {
		t.Fatal("HasAfter lost")
	}
	if !err.Retry.After.Equal(after) {
		t.Fatal("After lost")
	}
}

func TestErrorReasonContract(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.NotFound)
	if err.Reason != sdk.NotFound {
		t.Fatalf("Reason = %q, want %q", err.Reason, sdk.NotFound)
	}
	if got := sdk.ReasonOf(err); got != sdk.NotFound {
		t.Fatalf("ReasonOf = %q, want %q", got, sdk.NotFound)
	}
	if !sdk.IsReason(err, sdk.NotFound) {
		t.Fatal("IsReason should match")
	}
	if sdk.IsReason(err, sdk.Forbidden) {
		t.Fatal("IsReason should not match a different reason")
	}
}

func TestReasonWireValuesRemainStable(t *testing.T) {
	tests := []struct {
		name   string
		reason sdk.Reason
		wire   string
	}{
		{name: "invalid argument", reason: sdk.InvalidArgument, wire: "invalid_argument"},
		{name: "invalid cursor", reason: sdk.InvalidCursor, wire: "invalid_cursor"},
		{name: "unauthorized", reason: sdk.Unauthorized, wire: "unauthorized"},
		{name: "credentials expired", reason: sdk.CredentialsExpired, wire: "credentials_expired"},
		{name: "forbidden", reason: sdk.Forbidden, wire: "forbidden"},
		{name: "not found", reason: sdk.NotFound, wire: "not_found"},
		{name: "content unavailable", reason: sdk.ContentUnavailable, wire: "content_unavailable"},
		{name: "challenge required", reason: sdk.ChallengeRequired, wire: "challenge_required"},
		{name: "rate limited", reason: sdk.RateLimited, wire: "rate_limited"},
		{name: "upstream error", reason: sdk.UpstreamError, wire: "upstream_error"},
		{name: "upstream unavailable", reason: sdk.UpstreamUnavailable, wire: "upstream_unavailable"},
		{name: "malformed upstream response", reason: sdk.MalformedUpstreamResponse, wire: "malformed_upstream_response"},
		{name: "resource forbidden", reason: sdk.ResourceForbidden, wire: "resource_forbidden"},
		{name: "local state error", reason: sdk.LocalStateError, wire: "local_state_error"},
		{name: "removed setting", reason: sdk.RemovedSetting, wire: "removed_setting"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := string(test.reason); got != test.wire {
				t.Fatalf("reason wire value = %q, want %q", got, test.wire)
			}
		})
	}
}

func TestReasonHelpersPreserveWrappedAndEmptySemantics(t *testing.T) {
	if got := sdk.ReasonOf(nil); got != "" {
		t.Fatalf("ReasonOf(nil) = %q, want empty", got)
	}
	if got := sdk.ReasonOf(errors.New("ordinary error")); got != "" {
		t.Fatalf("ReasonOf(ordinary error) = %q, want empty", got)
	}
	if !sdk.IsReason(nil, "") {
		t.Fatal("IsReason(nil, empty) should preserve zero-value comparison semantics")
	}
	if sdk.IsReason(nil, sdk.NotFound) {
		t.Fatal("IsReason(nil, NotFound) should not match")
	}

	err := sdk.NewError("pixiv", "Artwork", sdk.NotFound)
	wrapped := fmt.Errorf("operation context: %w", err)
	if got := sdk.ReasonOf(wrapped); got != sdk.NotFound {
		t.Fatalf("ReasonOf(wrapped) = %q, want %q", got, sdk.NotFound)
	}
	if !sdk.IsReason(wrapped, sdk.NotFound) {
		t.Fatal("IsReason should inspect wrapped SDK errors")
	}
	if sdk.IsReason(wrapped, sdk.Forbidden) {
		t.Fatal("IsReason should reject a different wrapped reason")
	}
	if errors.Is(err, &sdk.Error{}) {
		t.Fatal("an empty reason sentinel must not match")
	}
}

func TestErrorFormattingUsesControlledFields(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.NotFound,
		sdk.WithDetail("status 404"),
		sdk.WithCause(errors.New("redacted local classification")),
		sdk.WithHTTPStatus(404),
		sdk.WithTransport(sdk.TransportHTTP),
		sdk.WithRetry(sdk.RetryAdvice{Safe: true, HasAfter: true}),
	)

	const want = "pixiv:Artwork: not_found: status 404: redacted local classification"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
