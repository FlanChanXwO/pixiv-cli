package sdk_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestErrorFormatIncludesProductOperationAndCode(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.CodeNotFound)
	msg := err.Error()
	if !strings.Contains(msg, "pixiv") || !strings.Contains(msg, "Artwork") || !strings.Contains(msg, "not_found") {
		t.Fatalf("Error() = %q", msg)
	}
}

func TestErrorDetailAndCause(t *testing.T) {
	cause := errors.New("redacted local classification")
	err := sdk.NewError("pixiv", "Artwork", sdk.CodeUpstreamError,
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

func TestErrorIsCode(t *testing.T) {
	err := sdk.NewError("pixiv", "SearchArtworks", sdk.CodeRateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true}))
	if !err.Retry.Safe {
		t.Fatal("retry advice lost")
	}
	if sdk.CodeOf(err) != sdk.CodeRateLimited {
		t.Fatalf("CodeOf = %q", sdk.CodeOf(err))
	}
	if !sdk.IsCode(err, sdk.CodeRateLimited) {
		t.Fatal("IsCode should match")
	}
	if sdk.IsCode(err, sdk.CodeNotFound) {
		t.Fatal("IsCode should not match other code")
	}
}

func TestErrorErrorsIsMatchesCodeSentinel(t *testing.T) {
	err := sdk.NewError("pixiv", "User", sdk.CodeForbidden)
	sentinel := &sdk.Error{Code: sdk.CodeForbidden}
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is should match code sentinel")
	}
	if errors.Is(err, &sdk.Error{Code: sdk.CodeNotFound}) {
		t.Fatal("errors.Is should not match different code")
	}
}

func TestErrorPreservesContextChain(t *testing.T) {
	err := sdk.NewError("pixiv", "Artwork", sdk.CodeUpstreamError,
		sdk.WithCause(context.Canceled),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context.Canceled not preserved in chain")
	}

	deadline := sdk.NewError("pixiv", "Artwork", sdk.CodeUpstreamError,
		sdk.WithCause(context.DeadlineExceeded),
	)
	if !errors.Is(deadline, context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded not preserved in chain")
	}
}

func TestErrorErrorsAs(t *testing.T) {
	wrapped := sdk.NewError("fanbox", "Post", sdk.CodeContentUnavailable)
	outer := sdk.NewError("fanbox", "Post", sdk.CodeContentUnavailable, sdk.WithCause(wrapped))
	var target *sdk.Error
	if !errors.As(outer, &target) {
		t.Fatal("errors.As should find *sdk.Error")
	}
	if target.Code != sdk.CodeContentUnavailable {
		t.Fatalf("As code = %q", target.Code)
	}
}

func TestErrorRetryAfter(t *testing.T) {
	after := time.Now().Add(30 * time.Second)
	err := sdk.NewError("pixiv", "SearchArtworks", sdk.CodeRateLimited,
		sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: after, HasAfter: true}),
	)
	if !err.Retry.HasAfter {
		t.Fatal("HasAfter lost")
	}
	if !err.Retry.After.Equal(after) {
		t.Fatal("After lost")
	}
}
