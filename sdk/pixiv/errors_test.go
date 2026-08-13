package pixiv_test

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestClassifyTransportPreservesTypedTransportKind(t *testing.T) {
	tests := []struct {
		name  string
		cause error
		want  sdk.Transport
	}{
		{
			name:  "dns",
			cause: &net.DNSError{Name: "pixiv.example", Err: "no such host"},
			want:  sdk.TransportDNS,
		},
		{
			name:  "tls",
			cause: x509.UnknownAuthorityError{},
			want:  sdk.TransportTLS,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.cause
			})
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			_, err = client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "transport"})
			if sdk.ReasonOf(err) != sdk.UpstreamUnavailable {
				t.Fatalf("Reason = %q, want %q", sdk.ReasonOf(err), sdk.UpstreamUnavailable)
			}
			classified, ok := err.(*sdk.Error)
			if !ok || classified.Transport != test.want {
				t.Fatalf("Transport = %q, want %q (err=%v)", classified.Transport, test.want, err)
			}
		})
	}
}
