package pixivapi_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
)

type transportTimeoutError struct{}

func (transportTimeoutError) Error() string   { return "transport timeout canary" }
func (transportTimeoutError) Timeout() bool   { return true }
func (transportTimeoutError) Temporary() bool { return true }

// TestTransportClassifiesSafeKinds 覆盖 TLS 包装错误、typed cause、proxy 优先级，
// 并固定错误文本不得泄露原始 cause。
func TestTransportClassifiesSafeKinds(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		kind pixivapi.TransportKind
	}{
		{name: "tls record", err: fmt.Errorf("wrapped: %w", tls.RecordHeaderError{Msg: "record canary"}), kind: pixivapi.TransportTLS},
		{name: "tls certificate", err: fmt.Errorf("wrapped: %w", &tls.CertificateVerificationError{Err: errors.New("certificate-detail-canary")}), kind: pixivapi.TransportTLS},
		{name: "tls alert", err: fmt.Errorf("wrapped: %w", tls.AlertError(42)), kind: pixivapi.TransportTLS},
		{name: "dns", err: fmt.Errorf("outer: %w", &net.DNSError{Name: "dns-canary.invalid", Err: "lookup canary"}), kind: pixivapi.TransportDNS},
		{name: "proxy", err: &url.Error{Op: "Get", URL: "https://proxy-user:proxy-pass@proxy.invalid/path?token=query-secret", Err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: syscall.ECONNREFUSED}}, kind: pixivapi.TransportProxy},
		{name: "refused", err: fmt.Errorf("outer: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}), kind: pixivapi.TransportConnectionRefused},
		{name: "reset", err: fmt.Errorf("outer: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}), kind: pixivapi.TransportConnectionReset},
		{name: "timeout", err: fmt.Errorf("outer: %w", &net.OpError{Op: "dial", Net: "tcp", Err: transportTimeoutError{}}), kind: pixivapi.TransportTimeout},
		{name: "unknown", err: errors.New("raw transport URL and credential canary"), kind: pixivapi.TransportUnknown},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			failure := pixivapi.Transport(test.err)
			if failure.Kind != pixivapi.FailureTransport || failure.TransportKind != test.kind {
				t.Fatalf("Transport() = %#v, want kind %q", failure, test.kind)
			}
			if errors.Unwrap(failure) != nil {
				t.Fatalf("errors.Unwrap(Transport()) = %v, want nil safe cause", errors.Unwrap(failure))
			}
			if failure.Error() != "pixiv upstream transport failed" {
				t.Fatalf("Failure.Error() = %q, want stable safe message", failure.Error())
			}
		})
	}
}

func TestTransportPreservesContextIdentity(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		failure := pixivapi.Transport(fmt.Errorf("context wrapper: %w", cause))
		if !errors.Is(failure, cause) || failure.TransportKind != "" {
			t.Fatalf("Transport(%v) = %#v", cause, failure)
		}
	}
}
