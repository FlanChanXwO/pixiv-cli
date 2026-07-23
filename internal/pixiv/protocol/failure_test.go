package protocol_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
)

// TestTransportClassifiesTLSRecordHeaderError 验证标准库 TLS record 错误不会落入 unknown。
func TestTransportClassifiesTLSRecordHeaderError(t *testing.T) {
	t.Parallel()

	failure := protocol.Transport(fmt.Errorf("wrapped: %w", tls.RecordHeaderError{Msg: "record canary"}))
	if failure.Kind != protocol.FailureTransport || failure.TransportKind != protocol.TransportTLS {
		t.Errorf("Transport() = %#v, want TLS transport failure", failure)
	}
}

// TestTransportClassifiesStableKinds 聚焦验证 typed cause、wrapper 穿透和 proxy 优先级。
func TestTransportClassifiesStableKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		kind protocol.TransportKind
	}{
		{
			name: "dns through fmt wrapper",
			err:  fmt.Errorf("outer: %w", &net.DNSError{Name: "dns-canary.invalid", Err: "lookup canary"}),
			kind: protocol.TransportDNS,
		},
		{
			name: "proxyconnect before inner refused",
			err: &url.Error{
				Op:  "Get",
				URL: "https://proxy-user:proxy-pass@proxy.invalid/path?token=query-secret",
				Err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: syscall.ECONNREFUSED},
			},
			kind: protocol.TransportProxy,
		},
		{
			name: "connection refused",
			err:  fmt.Errorf("outer: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
			kind: protocol.TransportConnectionRefused,
		},
		{
			name: "connection reset",
			err:  fmt.Errorf("outer: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}),
			kind: protocol.TransportConnectionReset,
		},
		{
			name: "typed network timeout",
			err:  fmt.Errorf("outer: %w", &net.OpError{Op: "dial", Net: "tcp", Err: transportTimeoutError{}}),
			kind: protocol.TransportTimeout,
		},
		{
			name: "unknown",
			err:  errors.New("raw transport URL and credential canary"),
			kind: protocol.TransportUnknown,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			failure := protocol.Transport(test.err)
			if failure.Kind != protocol.FailureTransport || failure.TransportKind != test.kind {
				t.Errorf("Transport() = %#v, want kind %q", failure, test.kind)
			}
			if errors.Unwrap(failure) != nil {
				t.Errorf("errors.Unwrap(Transport()) = %v, want nil safe cause", errors.Unwrap(failure))
			}
			if failure.Error() != "pixiv upstream transport failed" {
				t.Errorf("Failure.Error() = %q, want stable safe message", failure.Error())
			}
		})
	}
}

type transportTimeoutError struct{}

func (transportTimeoutError) Error() string   { return "transport timeout canary" }
func (transportTimeoutError) Timeout() bool   { return true }
func (transportTimeoutError) Temporary() bool { return true }

// TestTransportPreservesContextIdentity 验证安全分类不会替代取消和 deadline 的标准语义。
func TestTransportPreservesContextIdentity(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		failure := protocol.Transport(fmt.Errorf("context wrapper: %w", cause))
		if !errors.Is(failure, cause) {
			t.Errorf("errors.Is(Transport(%v), cause) = false", cause)
		}
		if failure.TransportKind != "" {
			t.Errorf("Transport(%v).TransportKind = %q, want empty context classification", cause, failure.TransportKind)
		}
	}
}

// TestTransportClassifiesTLSCertificateVerificationError 验证证书校验 wrapper 自身即可提供分类信号。
func TestTransportClassifiesTLSCertificateVerificationError(t *testing.T) {
	t.Parallel()

	failure := protocol.Transport(fmt.Errorf("wrapped: %w", &tls.CertificateVerificationError{Err: errors.New("certificate-detail-canary")}))
	if failure.Kind != protocol.FailureTransport || failure.TransportKind != protocol.TransportTLS {
		t.Errorf("Transport() = %#v, want TLS transport failure", failure)
	}
}

// TestTransportClassifiesTLSAlertError 验证标准库 TLS alert 保留 TLS 子类。
func TestTransportClassifiesTLSAlertError(t *testing.T) {
	t.Parallel()

	failure := protocol.Transport(fmt.Errorf("wrapped: %w", tls.AlertError(42)))
	if failure.Kind != protocol.FailureTransport || failure.TransportKind != protocol.TransportTLS {
		t.Errorf("Transport() = %#v, want TLS transport failure", failure)
	}
}
