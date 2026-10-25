package sdk_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestErrorContract(t *testing.T) {
	cause := errors.New("redacted local classification")
	after := time.Now().Add(30 * time.Second)
	err := sdk.NewError("pixiv", "Artwork", sdk.RateLimited,
		sdk.WithDetail("status 429"),
		sdk.WithCause(cause),
		sdk.WithHTTPStatus(429),
		sdk.WithTransport(sdk.TransportHTTP),
		sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: after, HasAfter: true}),
	)
	if got, want := err.Error(), "pixiv:Artwork: rate_limited: status 429: redacted local classification"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if err.Reason != sdk.RateLimited || err.HTTPStatus != 429 || err.Transport != sdk.TransportHTTP {
		t.Fatalf("error fields = %#v", err)
	}
	if !err.Retry.HasAfter || !err.Retry.After.Equal(after) || !err.Retry.Safe {
		t.Fatalf("retry advice = %#v", err.Retry)
	}
	if sdk.ReasonOf(err) != sdk.RateLimited || !sdk.IsReason(err, sdk.RateLimited) || sdk.IsReason(err, sdk.NotFound) {
		t.Fatalf("reason helpers disagree with %#v", err)
	}
	if !errors.Is(err, &sdk.Error{Reason: sdk.RateLimited}) || errors.Is(err, &sdk.Error{Reason: sdk.NotFound}) {
		t.Fatal("errors.Is reason sentinel mismatch")
	}
	var target *sdk.Error
	if !errors.As(err, &target) || target.Reason != sdk.RateLimited {
		t.Fatalf("errors.As = %#v", target)
	}
}

func TestErrorPreservesContextChain(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		err := sdk.NewError("pixiv", "Artwork", sdk.UpstreamError, sdk.WithCause(cause))
		if !errors.Is(err, cause) {
			t.Fatalf("%v not preserved in chain", cause)
		}
	}
	wrapped := fmt.Errorf("operation context: %w", sdk.NewError("pixiv", "Artwork", sdk.NotFound))
	if sdk.ReasonOf(wrapped) != sdk.NotFound || !sdk.IsReason(wrapped, sdk.NotFound) || sdk.IsReason(wrapped, sdk.Forbidden) {
		t.Fatalf("wrapped reason mismatch: %v", wrapped)
	}
}

func TestReasonHelpersPreserveEmptySemantics(t *testing.T) {
	if got := sdk.ReasonOf(nil); got != "" {
		t.Fatalf("ReasonOf(nil) = %q", got)
	}
	if got := sdk.ReasonOf(errors.New("ordinary error")); got != "" {
		t.Fatalf("ReasonOf(ordinary error) = %q", got)
	}
	if !sdk.IsReason(nil, "") || sdk.IsReason(nil, sdk.NotFound) {
		t.Fatal("IsReason nil semantics")
	}
	if errors.Is(sdk.NewError("pixiv", "Artwork", sdk.NotFound), &sdk.Error{}) {
		t.Fatal("an empty reason sentinel must not match")
	}
}

func TestReasonWireValuesRemainStable(t *testing.T) {
	for reason, wire := range map[sdk.Reason]string{
		sdk.InvalidArgument:           "invalid_argument",
		sdk.InvalidCursor:             "invalid_cursor",
		sdk.Unauthorized:              "unauthorized",
		sdk.CredentialsExpired:        "credentials_expired",
		sdk.Forbidden:                 "forbidden",
		sdk.NotFound:                  "not_found",
		sdk.ContentUnavailable:        "content_unavailable",
		sdk.ChallengeRequired:         "challenge_required",
		sdk.RateLimited:               "rate_limited",
		sdk.UpstreamError:             "upstream_error",
		sdk.UpstreamUnavailable:       "upstream_unavailable",
		sdk.MalformedUpstreamResponse: "malformed_upstream_response",
		sdk.ResourceForbidden:         "resource_forbidden",
		sdk.LocalStateError:           "local_state_error",
		sdk.RemovedSetting:            "removed_setting",
	} {
		if got := string(reason); got != wire {
			t.Fatalf("reason wire value = %q, want %q", got, wire)
		}
	}
}
