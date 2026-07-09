package pixiv

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientWithoutProxyIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("https_proxy", "http://env-proxy.invalid:7890")

	client, err := HTTPClient("")
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy)
}

func TestHTTPClientWithProxyUsesExplicitProxy(t *testing.T) {
	t.Setenv("https_proxy", "http://env-proxy.invalid:7890")

	client, err := HTTPClient("http://flag-proxy.invalid:7890")
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy)
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "app-api.pixiv.net"}}
	proxyURL, err := transport.Proxy(req)
	require.NoError(t, err)
	require.NotNil(t, proxyURL)
	assert.Equal(t, "http://flag-proxy.invalid:7890", proxyURL.String())
}
