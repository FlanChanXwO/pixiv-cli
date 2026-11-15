package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

// Failure carries sanitized protocol details for SDK error classification.
// Only context cancellation and deadlines remain in its unwrap chain; response
// bodies, URLs, headers, credentials, and upstream messages are omitted.
type Failure struct {
	Kind          FailureKind
	TransportKind TransportKind
	StatusCode    int
	RetryAfter    time.Duration
	HasRetryAfter bool
	cause         error
}

// TransportKind classifies transport failures from standard-library error types.
type TransportKind string

// TransportDNS and the related values describe the supported transport failures.
const (
	TransportDNS               TransportKind = "dns"
	TransportTLS               TransportKind = "tls"
	TransportProxy             TransportKind = "proxy"
	TransportConnectionRefused TransportKind = "connection_refused"
	TransportConnectionReset   TransportKind = "connection_reset"
	TransportTimeout           TransportKind = "timeout"
	TransportUnknown           TransportKind = "unknown"
)

// FailureKind classifies protocol failures for SDK mapping.
type FailureKind string

// FailureHTTPStatus and the related values distinguish HTTP, decoding, media policy,
// and transport failures.
const (
	FailureHTTPStatus        FailureKind = "http_status"
	FailureMalformed         FailureKind = "malformed_response"
	FailureResourceForbidden FailureKind = "resource_forbidden"
	FailureTransport         FailureKind = "transport"
)

// ErrMalformedResponse supports errors.Is for invalid upstream responses.
var ErrMalformedResponse = errors.New("pixiv upstream returned a malformed response")

// HTTPStatus constructs an HTTP failure without Retry-After.
func HTTPStatus(status int) Failure {
	return Failure{Kind: FailureHTTPStatus, StatusCode: status}
}

// HTTPStatusWithRetryAfter preserves a valid server wait when present is true.
func HTTPStatusWithRetryAfter(status int, retryAfter time.Duration, present bool) Failure {
	return Failure{
		Kind: FailureHTTPStatus, StatusCode: status, RetryAfter: retryAfter, HasRetryAfter: present,
	}
}

// MalformedResponse constructs a response decoding failure.
func MalformedResponse() Failure {
	return Failure{Kind: FailureMalformed}
}

// Transport sanitizes a transport error while preserving cancellation and deadlines.
func Transport(err error) Failure {
	var failure Failure
	if errors.As(err, &failure) {
		return failure
	}
	if errors.Is(err, context.Canceled) {
		return Failure{Kind: FailureTransport, cause: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Kind: FailureTransport, cause: context.DeadlineExceeded}
	}
	return Failure{Kind: FailureTransport, TransportKind: classifyTransportKind(err)}
}

// classifyTransportKind uses typed causes, never error text containing private URLs.
// Check proxyconnect before the connection-refused error it commonly wraps.
func classifyTransportKind(err error) TransportKind {
	var operationError *net.OpError
	if errors.As(err, &operationError) && operationError.Op == "proxyconnect" {
		return TransportProxy
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return TransportDNS
	}
	if isTLSTransportError(err) {
		return TransportTLS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return TransportConnectionRefused
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return TransportConnectionReset
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return TransportTimeout
	}
	return TransportUnknown
}

func isTLSTransportError(err error) bool {
	var certificateVerificationError *tls.CertificateVerificationError
	if errors.As(err, &certificateVerificationError) {
		return true
	}
	var unknownAuthorityError x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityError) {
		return true
	}
	var recordHeaderError tls.RecordHeaderError
	if errors.As(err, &recordHeaderError) {
		return true
	}
	var alertError tls.AlertError
	return errors.As(err, &alertError)
}

// Error reports fixed text without URLs, headers, bodies, or upstream messages.
func (e Failure) Error() string {
	switch e.Kind {
	case FailureHTTPStatus:
		return fmt.Sprintf("pixiv upstream returned HTTP status %d", e.StatusCode)
	case FailureMalformed:
		return ErrMalformedResponse.Error()
	case FailureResourceForbidden:
		return "pixiv resource URL is not allowed"
	default:
		return "pixiv upstream transport failed"
	}
}

// Unwrap returns the preserved context cancellation or deadline cause.
func (e Failure) Unwrap() error { return e.cause }

// Is supports matching ErrMalformedResponse.
func (e Failure) Is(target error) bool {
	return target == ErrMalformedResponse && e.Kind == FailureMalformed
}
